package main

import (
	"encoding/json"
	"errors"
	"flag"
	"github.com/fioprotocol/fio-go"
	fiohealth "github.com/fioprotocol/health"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"log"
	"os"
	"regexp"
	"strings"
)

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

	p2pAlerts       fiohealth.P2pAlerts
	apiAlerts       fiohealth.ApiAlerts
	telegramKey     string
	TelegramChannel string `yaml:"telegram_channel"`
	BaseUrl         string `yaml:"base_url"`
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

		// get alarm states, or create new
		b := make([]byte, 0)
		b, err = fiohealth.S3Get(c.bucket, c.prefix+"/json/api_health.json", c.Region)
		if err != nil {
			log.Println("error loading api alarm state, creating new: " + err.Error())
			c.apiAlerts.State = make(map[string]*fiohealth.ApiAlertState)
		} else {
			c.apiAlerts, err = fiohealth.UnmarshalApiAlerts(b)
			if err != nil {
				log.Println("error loading api alarm state, creating new: " + err.Error())
				c.apiAlerts.State = make(map[string]*fiohealth.ApiAlertState)
			}
		}
		b, err = fiohealth.S3Get(c.bucket, c.prefix+"/json/p2p_health.json", c.Region)
		if err != nil {
			log.Println("error loading p2p alarm state, creating new: " + err.Error())
			c.p2pAlerts.State = make(map[string]*fiohealth.P2pAlertState)
		} else {
			c.p2pAlerts, err = fiohealth.UnmarshalP2pAlerts(b)
			if err != nil {
				log.Println("error loading p2p alarm state, creating new: " + err.Error())
				c.p2pAlerts.State = make(map[string]*fiohealth.P2pAlertState)
			}
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

		j := make([]byte, 0)
		hf, err := os.Open(c.OutputDir + "/json/api_health.json")
		if err != nil {
			log.Println("error loading api alarm state, creating new: " + err.Error())
			c.apiAlerts.State = make(map[string]*fiohealth.ApiAlertState)
		} else {
			j, err = ioutil.ReadAll(hf)
			hf.Close()
			if err != nil {
				log.Println("error loading api alarm state, creating new: " + err.Error())
				c.apiAlerts.State = make(map[string]*fiohealth.ApiAlertState)
			}
		}
		if c.apiAlerts.State == nil {
			err = json.Unmarshal(j, &c.apiAlerts)
			if err != nil {
				log.Println("error loading api alarm state, creating new: " + err.Error())
				c.apiAlerts.State = make(map[string]*fiohealth.ApiAlertState)
			}
		}
		hf, err = os.Open(c.OutputDir + "/json/p2p_health.json")
		if err != nil {
			log.Println("error loading p2p alarm state, creating new: " + err.Error())
			c.p2pAlerts.State = make(map[string]*fiohealth.P2pAlertState)
		} else {
			j, err = ioutil.ReadAll(hf)
			hf.Close()
			if err != nil {
				log.Println("error loading p2p alarm state, creating new: " + err.Error())
				c.p2pAlerts.State = make(map[string]*fiohealth.P2pAlertState)
			}
		}
		if c.apiAlerts.State == nil {
			err = json.Unmarshal(j, &c.p2pAlerts)
			if err != nil {
				log.Println("error loading p2p alarm state, creating new: " + err.Error())
				c.p2pAlerts.State = make(map[string]*fiohealth.P2pAlertState)
			}
		}
	}
	return nil
}

func GetConfig() (*Config, error) {
	var (
		err               error
		configFile, confFile, geolite string
	)

	flag.StringVar(&confFile, "config", "config.yml", "yaml config file to load, can be local file, or S3 uri, or ENV var: CONFIG")
	flag.StringVar(&geolite, "db", "GeoLite2-Country.mmdb", "geo lite database to open")
	flag.Parse()

	switch true {
	case os.Getenv("CONFIG") != "":
		configFile = os.Getenv("CONFIG")
	case confFile != "":
		configFile = confFile
	}

	var y []byte
	switch true {
	case configFile == "":
		return nil, errors.New("cannot load config, no file specified")
	case strings.HasPrefix(configFile, "s3:"):
		y, err = fiohealth.S3GetUrl(configFile, "")
		if err != nil {
			return nil, err
		}
	default:
		f, err := os.Open(configFile)
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
	// the telegram key is sensitive, *only* allow via ENV var, should use encrypted parameter in AWS passed to lambda
	c.telegramKey = os.Getenv("TELEGRAM")

	return c, c.Validate()
}
