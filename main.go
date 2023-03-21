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

const templ = `Promethues Alert:
>状态:<font color=\"comment\">{{.Status}}</font>
>开始于:<font color=\"comment\">{{.StartsAt}}</font>
>Labels:
{{ range $key, $value := .Labels }}
	{{ $key }}:{{ $value }}
{{end}}
>Annotations:
{{ range $key, $value := .Annotations }}
	{{ $key }}:{{ $value }}
{{end}}
>详情:[点击查看]({{.GeneratorURL}})`

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

	for _, alert := range data.Alerts {
		klog.Infof("Alert: status=%s,Labels=%v,Annotations=%v", alert.Status, alert.Labels, alert.Annotations)

		if err := alertMsg(alert); err != nil {
			responseWithJson(w, http.StatusInternalServerError, "failed")
		}
	}

	responseWithJson(w, http.StatusOK, "success")
}

func alertMsg(alert template.Alert) error {
	msg := SendMsg{
		Msgtype: "markdown",
	}
	var doc bytes.Buffer
	t, err := gotemplate.New("alert").Parse(templ)
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
