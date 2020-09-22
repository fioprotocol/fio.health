package main

import (
	"bytes"
	"encoding/json"
	"github.com/aws/aws-lambda-go/lambda"
	fiohealth "github.com/fioprotocol/health"
	"github.com/fioprotocol/health/fhassets"
	"html/template"
	"io/ioutil"
	"log"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	if os.Getenv("LAMBDA_TASK_ROOT") != "" {
		lambda.Start(handler)
		return
	}
	log.Println(handler())
}

type History struct {
	File string `json:"file"`
	Date string `json:"date"`
	From string `json:"from"`

	sort int64
}

func handler() error {
	conf, err := GetConfig()
	if err != nil {
		return err
	}
	tmpl := template.New("Report")
	tmpl = template.Must(tmpl.Parse(fhassets.Report))
	final := fiohealth.FinalResult{
		Api:         checkApis(conf),
		P2p:         checkP2p(conf),
		Timestamp:   time.Now().UTC().Format(time.UnixDate),
		Description: conf.ReportTitle,
	}
	out := bytes.NewBuffer(nil)

	sort.Slice(final.P2p, func(i, j int) bool {
		if final.P2p[i].Score == final.P2p[j].Score {
			return final.P2p[i].Took > final.P2p[j].Took
		}
		return final.P2p[i].Score > final.P2p[j].Score
	})

	sort.Slice(final.Api, func(i, j int) bool {
		if final.Api[i].Score == final.Api[j].Score {
			return final.Api[i].RequestLatency > final.Api[j].RequestLatency
		}
		return final.Api[i].Score > final.Api[j].Score
	})
	err = tmpl.Execute(out, final)
	if err != nil {
		log.Println("template error:" + err.Error())
	}
	html := out.Bytes()

	now := time.Now().UTC()
	nowStr := strconv.FormatInt(now.UTC().Unix(), 10)
	nowFormat := now.Format(time.UnixDate)
	nowInt := now.Unix()
	geo, _ := fiohealth.MyGeo(conf.geolite)
	jIndex := make([]string, 0)
	hIndex := make([]History, 0)

	mkJson := func(payload interface{}) []byte {
		j := make([]byte, 0)
		switch payload.(type) {
		case []string:
			payload = append(payload.([]string), nowStr+".json")
			if len(payload.([]string)) > 144 {
				sort.Slice(payload.([]string), func(i, j int) bool {
					return payload.([]string)[i] > payload.([]string)[j]
				})
				payload = payload.([]string)[:144]
			}
			sort.Strings(payload.([]string))
		case []History:
			payload = append(payload.([]History), History{
				File: nowStr + ".html",
				Date: nowFormat,
				From: geo,
				sort: nowInt,
			})
			sort.Slice(payload.([]History), func(i, j int) bool {
				return payload.([]History)[i].sort > payload.([]History)[j].sort
			})
			// truncate html history
			if len(payload.([]History)) > 96 {
				payload = payload.([]History)[:96]
			}
		}
		j, _ = json.MarshalIndent(payload, "", "  ")
		return j
	}

	if conf.telegramKey != "" {
		for _, alert := range conf.apiAlerts.GetAlarms() {
			err = fiohealth.Notify(alert, conf.telegramKey, conf.TelegramChannel, conf.BaseUrl)
			if err != nil {
				log.Println(err)
				break
			}
		}

		for _, alert := range conf.p2pAlerts.GetAlarms() {
			err = fiohealth.Notify(alert, conf.telegramKey, conf.TelegramChannel, conf.BaseUrl)
			if err != nil {
				log.Println(err)
				break
			}
		}
	}

	combined := make([]fiohealth.FinalResult, 0)
	switch strings.HasPrefix(conf.OutputDir, "s3://") {
	case true:
		// get existing index, or create a new one
		index := func(dir string, kind string) {
			fTemp := make([]byte, 0)
			fTemp, err = fiohealth.S3Get(conf.bucket, conf.prefix+"/"+dir+"/index.json", conf.Region)
			j := make([]byte, 0)

			switch kind {
			case "json":
				err = json.Unmarshal(fTemp, &jIndex)
				if err != nil {
					jIndex = make([]string, 0)
				}
				combined = fiohealth.CombineS3Report(final, jIndex, conf.bucket, conf.prefix+"/"+dir, conf.Region)
				j = mkJson(jIndex)
			default:
				err = json.Unmarshal(fTemp, &hIndex)
				if err != nil {
					hIndex = make([]History, 0)
				}
				j = mkJson(hIndex)
			}
			err = fiohealth.S3Put(conf.bucket, conf.prefix+"/"+dir+"/"+"index.json", j, conf.Region)
			if err != nil {
				log.Println("could not write index: " + err.Error())
				return
			}
		}
		index("json", "json")
		index("history", "html")

		var j []byte
		j, err = json.MarshalIndent(final, "", "  ")
		if err != nil {
			return err
		}
		err = fiohealth.S3Put(conf.bucket, conf.prefix+"/json/"+nowStr+".json", j, conf.Region)
		if err != nil {
			return err
		}

		j, err = json.MarshalIndent(combined, "", "  ")
		if err != nil {
			return err
		}
		err = fiohealth.S3Put(conf.bucket, conf.prefix+"/json/report.json", j, conf.Region)
		if err != nil {
			return err
		}

		err = fiohealth.S3Put(conf.bucket, conf.prefix+"/history/"+nowStr+".html", html, conf.Region)
		if err != nil {
			return err
		}
		err = fiohealth.S3Put(conf.bucket, conf.prefix+"/index.html", html, conf.Region)
		if err != nil {
			return err
		}
		err = fhassets.WriteS3Assets(conf.DarkTheme, conf.bucket, conf.prefix, conf.Region)
		if err != nil {
			return err
		}
		err = fhassets.WriteS3Assets(conf.DarkTheme, conf.bucket, conf.prefix+"/history", conf.Region)
		if err != nil {
			return err
		}

		j, err = conf.apiAlerts.ToJson()
		if err != nil {
			return err
		}
		err = fiohealth.S3Put(conf.bucket, conf.prefix+"/json/api_health.json", j, conf.Region)
		if err != nil {
			return err
		}
		j, err = conf.p2pAlerts.ToJson()
		if err != nil {
			return err
		}
		err = fiohealth.S3Put(conf.bucket, conf.prefix+"/json/p2p_health.json", j, conf.Region)
		if err != nil {
			return err
		}

	default:
		index := func(dir string, kind string) error {
			j := make([]byte, 0)
			f, err := os.OpenFile(conf.OutputDir+"/"+dir+"/index.json", os.O_RDWR|os.O_CREATE, 0644)
			if err != nil {
				return err
			}
			defer f.Close()
			b, err := ioutil.ReadAll(f)
			switch kind {
			case "json":
				if len(b) > 0 {
					err = json.Unmarshal(b, &jIndex)
					if err != nil {
						return err
					}
				}
				combined = fiohealth.CombineReport(final, jIndex, conf.OutputDir+"/json")
				j = mkJson(jIndex)
			case "html":
				if len(b) > 0 {
					err = json.Unmarshal(b, &hIndex)
					if err != nil {
						return err
					}
				}
				j = mkJson(hIndex)
			}
			_ = f.Truncate(0)
			_, err = f.WriteAt(j, 0)
			return err
		}
		err = index("json", "json")
		if err != nil {
			return err
		}
		err = index("history", "html")
		if err != nil {
			return err
		}

		f := &os.File{}
		var j []byte
		j, err = json.MarshalIndent(final, "", "  ")
		if err != nil {
			return err
		}
		f, err = os.OpenFile(conf.OutputDir+"/json/"+nowStr+".json", os.O_TRUNC|os.O_WRONLY|os.O_CREATE, 0644)
		if err != nil {
			return err
		}
		_, err = f.Write(j)
		if err != nil {
			return err
		}
		f.Close()

		j, err = json.MarshalIndent(combined, "", "  ")
		if err != nil {
			return err
		}
		f, err = os.OpenFile(conf.OutputDir+"/json/report.json", os.O_TRUNC|os.O_WRONLY|os.O_CREATE, 0644)
		if err != nil {
			return err
		}
		_, err = f.Write(j)
		if err != nil {
			return err
		}
		f.Close()

		f, err = os.OpenFile(conf.OutputDir+"/index.html", os.O_TRUNC|os.O_WRONLY|os.O_CREATE, 0644)
		if err != nil {
			return err
		}
		_, err = f.Write(html)
		if err != nil {
			return err
		}
		f.Close()

		f, err = os.OpenFile(conf.OutputDir+"/history/"+nowStr+".html", os.O_TRUNC|os.O_WRONLY|os.O_CREATE, 0644)
		if err != nil {
			return err
		}
		_, err = f.Write(html)
		if err != nil {
			return err
		}
		f.Close()

		j, err = conf.apiAlerts.ToJson()
		if err != nil {
			return err
		}
		f, err = os.OpenFile(conf.OutputDir+"/json/api_health.json", os.O_TRUNC|os.O_WRONLY|os.O_CREATE, 0644)
		if err != nil {
			return err
		}
		_, err = f.Write(j)
		if err != nil {
			return err
		}
		f.Close()

		j, err = conf.p2pAlerts.ToJson()
		if err != nil {
			return err
		}
		f, err = os.OpenFile(conf.OutputDir+"/json/p2p_health.json", os.O_TRUNC|os.O_WRONLY|os.O_CREATE, 0644)
		if err != nil {
			return err
		}
		_, err = f.Write(j)
		if err != nil {
			return err
		}
		f.Close()

		err = fhassets.WriteLocalAssets(conf.DarkTheme, conf.OutputDir)
		if err != nil {
			return err
		}
		err = fhassets.WriteLocalAssets(conf.DarkTheme, conf.OutputDir+"/history")
		if err != nil {
			return err
		}
	}
	return nil
}

