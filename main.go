package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"net/http"
	gotemplate "text/template"

	"github.com/gorilla/mux"
	"github.com/prometheus/alertmanager/template"
	"k8s.io/klog"
)

const templ = `{{ $var := .ExternalURL}}{{ range $k,$v:=.Alerts }}{{if eq $v.Status "resolved"}}[PROMETHEUS-恢复信息]({{$v.GeneratorURL}})
> **[{{$v.Labels.alertname}}]({{$var}})**
> <font color="info">告警级别:</font> {{$v.Labels.severity}}
> <font color="info">开始时间:</font> {{$v.StartsAt.Local}}
> <font color="info">结束时间:</font> {{$v.EndsAt.Local}}
> <font color="info">故障主机IP:</font> {{$v.Labels.instance}}
> <font color="info">**{{$v.annotations.description}}**</font>
{{else}}
[PROMETHEUS-告警信息]({{$v.GeneratorURL}})
> **[{{$v.Labels.alertname}}]({{$var}})**
> <font color="warning">告警级别:</font> {{$v.Labels.severity}}
> <font color="warning">开始时间:</font> {{$v.StartsAt.Local}}
> <font color="warning">结束时间:</font> {{$v.EndsAt.Local}}
> <font color="warning">故障主机IP:</font> {{$v.Labels.instance}}
> <font color="warning">**{{$v.Annotations.description}}**</font>
{{end}}{{ end }}
{{ $urimsg:=""}}{{ range $key,$value:=.CommonLabels }}{{$urimsg =  print $urimsg $key "%3D%22" $value "%22%2C" }}{{end}}[*** 点我屏蔽该告警]({{$var}}/#/silences/new?filter=%7B{{SplitString $urimsg 0 -3}}%7D)`

// Parameter
var wechatWorkURL string

// Msg to wechat work
type SendMsg struct {
	Msgtype  string      `json:"msgtype"`
	Markdown interface{} `json:"markdown"`
}

type MsgContent struct {
	Content string `json:"content"`
}

func init() {
	flag.StringVar(&wechatWorkURL, "url", "", "url for wechat work robot.")
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

func alertMsg(alert template.Data) error {
	msg := SendMsg{
		Msgtype: "markdown",
	}
	var doc bytes.Buffer
	t, err := gotemplate.New("alert").Funcs(gotemplate.FuncMap{"SplitString": func(pstring string, start int, stop int) string {
		if stop < 0 {
			return pstring[start : len(pstring)+stop]
		}
		return pstring[start:stop]
	}}).Parse(templ)
	if err != nil {
		klog.Errorf("Webhook: initial go template error: %v", err.Error())
		return err
	}

	if err := t.Execute(&doc, alert); err != nil {
		klog.Errorf("Webhook: go template execute error: %v", err.Error())
		return err
	}
	msg.Markdown = &MsgContent{Content: doc.String()}

	if err := sendToWechatWork(msg); err != nil {
		return err
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
	router := mux.NewRouter()
	router.HandleFunc("/healthz", healthz)
	router.HandleFunc("/webhook", webhook)

	listenAddress := ":8080"
	klog.Fatal(http.ListenAndServe(listenAddress, router))
}
