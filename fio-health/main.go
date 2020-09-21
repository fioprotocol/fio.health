package main

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/eoscanada/eos-go"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/fioprotocol/fio-go"
	"github.com/fioprotocol/fio-go/eos/p2p"
	fiohealth "github.com/fioprotocol/health"
	"github.com/fioprotocol/health/fhassets"
	"gopkg.in/yaml.v2"
	"html/template"
	"io/ioutil"
	"log"
	"math"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

func main() {
	log.SetFlags(log.LstdFlags|log.Lshortfile)
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
				File: nowStr +".html",
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
				j  = mkJson(jIndex)
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

type Config struct {
	ReportTitle string   `yaml:"report_title"`
	ChainId     string   `yaml:"chain_id"`
	ApiNodes    []string `yaml:"api_nodes"`
	P2pNodes    []string `yaml:"p2p_nodes"`
	OutputDir   string   `yaml:"output_dir"`
	Region      string   `yaml:"region"`
	DarkTheme   bool     `yaml:"dark_theme"`

	bucket  string
	prefix  string
	geolite string
}


func (c *Config) Validate() error {
	if c.ReportTitle == "" {
		return errors.New("report title is required")
	}
	switch c.ChainId {
	case "mainnet":
		c.ChainId = fio.ChainIdMainnet
	case "testnet":
		c.ChainId = fio.ChainIdTestnet
	case "":
		return errors.New("chain id is required")
	}

	switch 0 {
	case len(c.ApiNodes):
		return errors.New("no api nodes supplied")
	case len(c.P2pNodes):
		return errors.New("no p2p nodes supplied")
	}

	formatErrs := make([]string, 0)
	for i := range c.ApiNodes {
		if !strings.HasPrefix(c.ApiNodes[i], "http") {
			formatErrs = append(formatErrs, "malformed api'"+c.ApiNodes[i]+"' missing http(s) prefix")
		}
		c.ApiNodes[i] = strings.TrimRight(c.ApiNodes[i], "/")
	}
	r := regexp.MustCompile(`\w+:\d+`)
	for i := range c.P2pNodes {
		if !r.MatchString(c.P2pNodes[i]) {
			formatErrs = append(formatErrs, "malformed p2p '"+c.P2pNodes[i]+"' should be name:port")
		}
	}
	if len(formatErrs) > 0 {
		return errors.New(strings.Join(formatErrs, ", "))
	}

	if c.OutputDir == "" {
		c.OutputDir = "."
	}
	if len(c.OutputDir) > 1 && strings.HasSuffix(c.OutputDir, "/") {
		c.OutputDir = c.OutputDir[:len(c.OutputDir)-2]
	}

	switch strings.HasPrefix(c.OutputDir, "s3://") {
	case true:
		parts := strings.Split(c.OutputDir, "/")
		if len(parts) < 4 {
			return errors.New("malformed s3 url for output dir")
		}
		c.bucket = parts[2]
		c.prefix = strings.Join(parts[3:len(parts)], "/")

		err := fiohealth.S3Put(c.bucket, c.prefix+"/.write_test", []byte("test"), c.Region)
		if err != nil {
			return errors.New("test s3 write: " + err.Error())
		}
	default:
		f, err := os.OpenFile(c.OutputDir+"/.write_test", os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
		if err != nil {
			return errors.New("test output directory write: " + err.Error())
		}
		defer f.Close()
		_, err = f.WriteString("test")
		if err != nil {
			return errors.New("test output directory write: " + err.Error())
		}
		err = os.MkdirAll(c.OutputDir+"/json", 0755)
		if err != nil {
			return errors.New("could not create output dir for json reports: " + err.Error())
		}
		err = os.MkdirAll(c.OutputDir+"/history", 0755)
		if err != nil {
			return errors.New("could not create output dir for historical html reports: " + err.Error())
		}
	}

	return nil
}

func GetConfig() (*Config, error) {
	var (
		err               error
		confFile, geolite string
	)

	flag.StringVar(&confFile, "conf", "config.yml", "yaml config file to load, can be local file, or S3 uri, or ENV var: CONFIG")
	flag.StringVar(&geolite, "db", "GeoLite2-Country.mmdb", "geo lite database to open")
	flag.Parse()

	if os.Getenv("CONFIG") != "" {
		confFile = os.Getenv("CONFIG")
	}

	var y []byte
	switch true {
	case confFile == "":
		return nil, errors.New("cannot load config, no file specified")
	case strings.HasPrefix(confFile, "s3:"):
		y, err = fiohealth.S3GetUrl(confFile, "")
		if err != nil {
			return nil, err
		}
	default:
		f, err := os.Open(confFile)
		if err != nil {
			return nil, err
		}
		y, err = ioutil.ReadAll(f)
		if err != nil {
			return nil, err
		}
	}
	c := &Config{}
	err = yaml.Unmarshal(y, c)
	if err != nil {
		return c, err
	}

	if strings.HasPrefix(c.OutputDir, "s3://") && c.Region == "" {
		c.Region = "us-east-1"
	}
	c.geolite = geolite

	return c, c.Validate()
}


func checkP2p(conf *Config) (report []fiohealth.P2pResult) {
	geo, err := fiohealth.MyGeo(conf.geolite)
	if err != nil {
		log.Println(err)
	}
	results := make([]fiohealth.P2pResult, len(conf.P2pNodes))
	wg := sync.WaitGroup{}
	wg.Add(len(conf.P2pNodes))
	for i := range conf.P2pNodes {
		go func(i int) {
			results[i] = getBlockP2p(conf.P2pNodes[i], geo, conf)
			wg.Done()
		}(i)
	}
	wg.Wait()
	return results
}

func getBlockP2p(p2pnode string, geo string, conf *Config) fiohealth.P2pResult {
	started := time.Now().UTC()
	r := fiohealth.P2pResult{Type: "p2p", Peer: p2pnode, FromGeo: geo, TimeStamp: time.Now().UTC().Unix()}
	cid, err := hexutil.Decode("0x" + conf.ChainId)
	if err != nil {
		log.Fatal(err)
	}
	peer := p2p.NewOutgoingPeer(p2pnode, "healthcheck", &p2p.HandshakeInfo{
		ChainID:      cid,
		HeadBlockNum: 1,
	})
	peer.SetConnectionTimeout(5 * time.Second)
	client := p2p.NewClient(
		peer,
		false,
	)
	client.SetReadTimeout(time.Second)
	blockHandler := p2p.HandlerFunc(func(envelope *p2p.Envelope) {
		r.Reachable = true
		name, _ := envelope.Packet.Type.Name()
		switch name {
		case "GoAway":
			why := &eos.GoAwayMessage{}
			err := eos.UnmarshalBinary(envelope.Packet.Payload, why)
			if err != nil {
				log.Println(err)
				r.ErrMsg = err.Error()
			}
			r.Score += 1
			r.ErrMsg = why.String() + " " + why.Reason.String()
		case "SignedBlock":
			block := &eos.SignedBlock{}
			err := eos.UnmarshalBinary(envelope.Packet.Payload, block)
			if err != nil {
				r.ErrMsg = err.Error()
				return
			}
			delta := time.Now().UTC().Sub(block.Timestamp.Time)
			if delta.Seconds() < 30 {
				r.Healthy = true
			} else {
				r.Score += 1
			}
			r.HeadBlockLatency = delta.Milliseconds()
			_ = client.CloseConnection()
		}
	})
	client.RegisterHandler(blockHandler)
	go func() {
		time.Sleep(2 * time.Second)
		_ = client.CloseConnection()
	}()
	err = client.Start()
	if err != nil {
		if !r.Healthy {
			emsg := err.Error()
			switch true {
			case strings.HasSuffix(emsg, "err: EOF"):
				emsg = "too many peers? (EOF)"
				r.Reachable = true
			case strings.HasSuffix(emsg, "use of closed network connection"):
				emsg = "too slow to respond"
			case strings.HasSuffix(emsg, "timeout"):
				emsg = "connection timeout"
			case strings.HasSuffix(emsg, "no such host"):
				emsg = "name lookup failed"
			case strings.HasSuffix(emsg, "connection reset by peer"):
				emsg = "connection reset"
			case strings.HasSuffix(emsg, "connection refused"):
				emsg = "connection refused"
			}
			r.ErrMsg = emsg
			r.Score += 1
		}
	}
	r.Took = time.Now().UTC().Sub(started).Milliseconds() / 1000
	return r
}

func checkApis(conf *Config) (report []fiohealth.Result) {
	myIpAddr, err := fiohealth.MyGeo(conf.geolite)
	if err != nil {
		log.Fatal(err)
	}
	wg := sync.WaitGroup{}
	wg.Add(len(conf.ApiNodes))
	results := make([]fiohealth.Result, len(conf.ApiNodes))
	for i, a := range conf.ApiNodes {
		go func(i int, a string) {
			defer wg.Done()
			results[i] = fiohealth.Result{
				Type:      "api",
				Node:      a,
				TimeStamp: time.Now().UTC().Unix(),
				FromGeo:   myIpAddr,
			}
			api, _, err := fio.NewConnection(nil, a)
			if err != nil {
				log.Println(a, "new connection", err.Error())
				emsg := err.Error()
				switch true {
				case strings.HasSuffix(emsg, "timeout"):
					emsg = "connection timeout"
				case strings.HasSuffix(emsg, "no such host"):
					emsg = "name lookup failed"
				case strings.HasSuffix(emsg, "connection reset by peer"):
					emsg = "connection refused"
				}
				results[i].HadError = true
				results[i].Error = emsg
				results[i].ErrorFor = "initial connection"
				results[i].Score += 10
				return
			}
			before := time.Now().UTC()
			gi, err := api.GetInfo()
			now := time.Now().UTC()
			results[i].RequestLatency = now.Sub(before).Milliseconds()
			if err != nil {
				log.Println(a, "get info", err.Error())
				results[i].HadError = true
				results[i].Error = err.Error()
				results[i].ErrorFor = "get info"
				results[i].Score += 10
				return
			}
			results[i].HeadBlockLatency = now.Sub(gi.HeadBlockTime.Time).Milliseconds()
			results[i].NodeVer = gi.ServerVersionString
			if gi.HeadBlockTime.Time.Before(time.Now().UTC().Add(-30 * time.Second)) {
				log.Println(a, "is not synced!")
				results[i].HadError = true
				results[i].Error = fmt.Sprintf("node head block is behind by %.2f", now.Sub(gi.HeadBlockTime.Time).Seconds())
				results[i].ErrorFor = "get info"
				results[i].Score += 1
				//return
			}
			if gi.ChainID.String() != conf.ChainId {
				log.Println(a, "Wrong chain!")
				results[i].HadError = true
				results[i].Error = "wrong chain"
				results[i].ErrorFor = "get info"
				results[i].Score += 5
			}
			_, err = api.GetBlockByNum(gi.LastIrreversibleBlockNum)
			if err != nil {
				log.Println(a, "get block", err.Error())
				results[i].HadError = true
				results[i].Error = err.Error()
				results[i].ErrorFor = "get block"
				results[i].Score += 10
				return
			}

			notes := make([]string, 0)

			if finding, found := fiohealth.TestTls(api.BaseURL); found {
				notes = append(notes, finding)
				results[i].Score += 1
			} else if strings.HasPrefix(api.BaseURL, "https") {
				results[i].TlsCipherOk = true
			}

			// going to use native http lib here to get access to response headers and TLS info:
			resp, err := http.Get(api.BaseURL + "/v1/chain/get_producer_schedule")
			if err != nil {
				log.Println(a, "producer schedule", err.Error())
				results[i].HadError = true
				results[i].Error = err.Error()
				results[i].ErrorFor = "get producer schedule"
				results[i].Score += 10
			}
			if resp == nil {
				return
			}
			_ = resp.Body.Close()

			if resp.Header.Get("Access-Control-Allow-Origin") == "*" {
				results[i].PermissiveCors = true
			} else {
				results[i].Score += 1
			}

			if resp.TLS != nil {
				appendIf := func(n string) {
					if n != "" {
						notes = append(notes, n)
					}
				}

				if resp.TLS.Version >= tls.VersionTLS12 {
					results[i].TlsVerOk = true
				} else {
					appendIf("negotiated TLS version < 1.2")
					results[i].Score += 1
				}
				if len(resp.TLS.PeerCertificates) > 0 && resp.TLS.PeerCertificates[0] != nil {
					expires := resp.TLS.PeerCertificates[0].NotAfter.Sub(time.Now().UTC()).Hours() / 24
					if expires < 30 {
						appendIf(fmt.Sprintf("expires in %d days", int64(math.Round(expires))))
						results[i].Score += .1
					}
				}
				results[i].TlsNote = strings.Join(notes, ", ")
			} else {
				results[i].TlsNote = "TLS not enabled"
				results[i].Score += 1
			}

			// should always get an error, if not network/producer api is exposed:
			_, err = api.GetNetConnections()
			if err == nil {
				log.Println(a, "net api")
				results[i].NetExposed = true
				results[i].Score += 3
			}
			_, err = api.IsProducerPaused()
			if err == nil {
				log.Println(a, "producer api")
				results[i].ProducerExposed = true
				results[i].Score += 3
			}
		}(i, a)
	}
	wg.Wait()
	return results
}
