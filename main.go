package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"strings"
	gotemplate "text/template"

	"github.com/gorilla/mux"
	"github.com/prometheus/alertmanager/template"
	"k8s.io/klog"
)

// const templ = `{{ $var := .ExternalURL}}{{ range $k,$v:=.Alerts }}{{if eq $v.Status "resolved"}}[PROMETHEUS-恢复信息]({{$v.GeneratorURL}})
// > **[{{$v.Labels.alertname}}]({{$var}})**
// > <font color="info">告警级别:</font> {{$v.Labels.severity}}
// > <font color="info">开始时间:</font> {{$v.StartsAt.Local}}
// > <font color="info">结束时间:</font> {{$v.EndsAt.Local}}
// > <font color="info">故障主机IP:</font> {{$v.Labels.instance}}
// > <font color="info">**{{$v.Annotations.description}}**</font>
// {{else}}
// [PROMETHEUS-告警信息]({{$v.GeneratorURL}})
// > **[{{$v.Labels.alertname}}]({{$var}})**
// > <font color="warning">告警级别:</font> {{$v.Labels.severity}}
// > <font color="warning">开始时间:</font> {{$v.StartsAt.Local}}
// > <font color="warning">结束时间:</font> {{$v.EndsAt.Local}}
// > <font color="warning">故障主机IP:</font> {{$v.Labels.instance}}
// > <font color="warning">**{{$v.Annotations.description}}**</font>
// {{end}}{{ end }}
// {{ $urimsg:=""}}{{ range $key,$value:=.CommonLabels }}{{$urimsg =  print $urimsg $key "%3D%22" $value "%22%2C" }}{{end}}[*** 点我屏蔽该告警]({{$var}}/#/silences/new?filter=%7B{{SplitString $urimsg 0 -3}}%7D)`

const templ1 = `{{if eq .Status "resolved"}}[PROMETHEUS-恢复信息]({{.GeneratorURL}})
> **[{{.Labels.alertname}}]({{.ExternalURL}})**
> <font color="info">告警级别:</font> {{.Labels.severity}}
> <font color="info">开始时间:</font> {{.StartsAt.Local}}
> <font color="info">结束时间:</font> {{.EndsAt.Local}}
> <font color="info">故障主机IP:</font> {{.Labels.instance}}
> <font color="info">**{{.Annotations.description}}**</font>
{{if ne .Kibana ""}}[**【{{.AppName}}日志】**]({{.Kibana}}){{end}}
{{if ne .PodNetworking ""}}[**【{{.PodName}}网络】**]({{.PodNetworking}}){{end}}{{if ne .AppNetworking ""}}[**【{{.AppName}}网络】**]({{.AppNetworking}}){{end}}
{{if ne .PodComputeResource ""}}[**【{{.PodName}}资源】**]({{.PodComputeResource}}){{end}}{{if ne .AppComputeResource ""}}[**【{{.AppName}}资源】**]({{.AppComputeResource}}){{end}}
{{else}}
[PROMETHEUS-告警信息]({{.GeneratorURL}})
> **[{{.Labels.alertname}}]({{.ExternalURL}})**
> <font color="warning">告警级别:</font> {{.Labels.severity}}
> <font color="warning">开始时间:</font> {{.StartsAt.Local}}
> <font color="warning">结束时间:</font> {{.EndsAt.Local}}
> <font color="warning">故障主机IP:</font> {{.Labels.instance}}
> <font color="warning">**{{.Annotations.description}}**</font>
{{if ne .Kibana ""}}[**【{{.AppName}}日志】**]({{.Kibana}}){{end}}
{{if ne .PodNetworking ""}}[**【{{.PodName}}网络】**]({{.PodNetworking}}){{end}}{{if ne .AppNetworking ""}}[**【{{.AppName}}网络】**]({{.AppNetworking}}){{end}}
{{if ne .PodComputeResource ""}}[**【{{.PodName}}资源】**]({{.PodComputeResource}}){{end}}{{if ne .AppComputeResource ""}}[**【{{.AppName}}资源】**]({{.AppComputeResource}}){{end}}
{{end}}
{{ $urimsg:=""}}{{ range $key,$value:=.Labels }}{{$urimsg =  print $urimsg $key "%3D%22" $value "%22%2C" }}{{end}}[*** 点我屏蔽该告警]({{.ExternalURL}}/#/silences/new?filter=%7B{{SplitString $urimsg 0 -3}}%7D)`

// 支持查看kibana日志的namespace列表
var namespaces = []string{"sit13", "uat14", "uat15", "im", "im2", "eeo", "services"}

// Parameter
var wechatWorkURL string
var endpointAlertInfo string

// 监控与日志信息
type AppGrfanaKibanaInfo struct {
	ParamGetAppGrfanaKibanaInfo
	AppNetworking      string `json:"app_networking"`
	PodNetworking      string `json:"pod_networking"`
	AppComputeResource string `json:"app_compute_resource"`
	PodComputeResource string `json:"pod_compute_resource"`
	Kibana             string `json:"kibana"`
}

type ParamGetAppGrfanaKibanaInfo struct {
	ClusterName string `json:"cluster_name"`
	AppEnv      string `json:"app_env"`
	AppName     string `json:"app_name"`
	PodName     string `json:"pod_name"`
}

type TemplateData struct {
	AppGrfanaKibanaInfo
	template.Alert
	ExternalURL string
}

// Msg to wechat work
type SendMsg struct {
	Msgtype  string      `json:"msgtype"`
	Markdown interface{} `json:"markdown"`
}

type MsgContent struct {
	Content string `json:"content"`
}

func init() {
	flag.StringVar(&wechatWorkURL, "url", "", "微信机器人地址")
	flag.StringVar(&endpointAlertInfo, "info", "http://endpoint-infos.services", "获取grafna与kibana url信息的地址")
}

func responseWithJson(w http.ResponseWriter, code int, payload interface{}) {
	response, _ := json.Marshal(payload)
	w.WriteHeader(code)
	w.Write(response)
}

func healthz(w http.ResponseWriter, r *http.Request) {
	responseWithJson(w, http.StatusOK, "OK!")
}

func webhook(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	data := template.Data{}

	if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
		responseWithJson(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := alertMsg(data); err != nil {
		responseWithJson(w, http.StatusInternalServerError, "failed")
	}

	responseWithJson(w, http.StatusOK, "success")
}

func getInfos(podName string, namespace string, externalURL string) (*AppGrfanaKibanaInfo, error) {
	deployName := ""
	splitStr := strings.Split(podName, "-")
	if len(splitStr) >= 3 {
		deployName = strings.Join(splitStr[:len(splitStr)-2], "-")
	}
	params := ParamGetAppGrfanaKibanaInfo{
		ClusterName: getClusterName(externalURL),
		AppEnv:      namespace,
		AppName:     deployName,
		PodName:     podName,
	}

	data, err := json.Marshal(params)
	if err != nil {
		klog.Errorf("getInfos: podName: %s, namespace: %s, externalURL: %s", podName, namespace, externalURL)
		return nil, err
	}

	var info AppGrfanaKibanaInfo
	req_url := fmt.Sprintf("%s/api/v1/endpoint_info", endpointAlertInfo)
	if resp, err := http.Post(req_url, "application/json", bytes.NewBuffer(data)); err != nil {
		klog.Errorf("请求%s 错误: %v", req_url, err)
	} else {
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			if data, e := io.ReadAll(resp.Body); e != nil {
				klog.Errorf("请求%s 解析resp错误: %v", req_url, e)
			} else {
				klog.Errorf("请求%s 错误 statusCode: %d resp: %s", req_url, resp.StatusCode, string(data[:]))
			}
		} else {
			if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
				klog.Errorln(err)
			}
		}
	}

	info.ParamGetAppGrfanaKibanaInfo = params
	return &info, nil
}

func getClusterName(url string) string {
	start := strings.Index(url, "https://alert-") + len("https://alert-")
	end := strings.Index(url, ".eeo-inc.com")
	return fmt.Sprintf("eeo-%s", url[start:end])
}

func alertMsg(data template.Data) error {
	msg := SendMsg{
		Msgtype: "markdown",
	}
	var doc bytes.Buffer
	t, err := gotemplate.New("alert").Funcs(gotemplate.FuncMap{"SplitString": func(pstring string, start int, stop int) string {
		if stop < 0 {
			return pstring[start : len(pstring)+stop]
		}
		return pstring[start:stop]
	}}).Parse(templ1)
	if err != nil {
		klog.Errorf("Webhook: initial go template error: %v", err.Error())
		return err
	}

	for _, alert := range data.Alerts {
		ns := alert.Labels["namespace"]
		alertData := TemplateData{Alert: alert, ExternalURL: data.ExternalURL}
		if info, err := getInfos(alert.Labels["pod"], ns, data.ExternalURL); err != nil {
			klog.Errorln(err)
		} else {
			nsstr := strings.Join(namespaces, ",")
			if !strings.Contains(nsstr, ns) {
				info.Kibana = ""
			}
			alertData.AppGrfanaKibanaInfo = *info
		}
		jsondata, _ := json.Marshal(alertData)
		klog.Infof("接受到的报警内容: %s", string(jsondata))
		// t.Execute(os.Stderr, alertData)
		if err := t.Execute(&doc, alertData); err != nil {
			klog.Errorf("Webhook: go template execute error: %v", err.Error())
			return err
		}
		msg.Markdown = &MsgContent{Content: doc.String()}

		if err := sendToWechatWork(msg); err != nil {
			klog.Errorf("发送报错: %v", err.Error())
			return err
		}
	}
	return nil
}

func sendToWechatWork(msg interface{}) error {
	jsonBytes, err := json.Marshal(msg)
	if err != nil {
		klog.Errorf("sendToWechatWork: json marshal error: %v", err.Error())
		return err
	}

	reader := bytes.NewReader(jsonBytes)
	url := wechatWorkURL
	request, err := http.NewRequest("POST", url, reader)
	if err != nil {
		klog.Errorf("sendToWechatWork: http new request error: %v", err.Error())
		return err
	}

	request.Header.Set("Content-Type", "application/json")
	client := http.Client{}
	_, err = client.Do(request)
	if err != nil {
		klog.Errorf("sendToWechatWork: http post request error: %v", err.Error())
		return err
	}

	return nil
}

func main() {
	flag.Parse()
	klog.Infof("alert url: %s \n info url: %s", wechatWorkURL, endpointAlertInfo)
	router := mux.NewRouter()
	router.HandleFunc("/healthz", healthz)
	router.HandleFunc("/webhook", webhook)

	listenAddress := ":8080"
	klog.Fatal(http.ListenAndServe(listenAddress, router))
}
