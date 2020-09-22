package fiohealth

import (
	"encoding/json"
	"github.com/oschwald/maxminddb-golang"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"sort"
)

type FinalResult struct {
	Api         []Result    `json:"api"`
	P2p         []P2pResult `json:"p2p"`
	Timestamp   string      `json:"timestamp"`
	Description string      `json:"description"`
}

// Result is the output from an API health check
type Result struct {
	Type             string  `json:"type"`
	Node             string  `json:"node"`
	NodeVer          string  `json:"node_ver"`
	TimeStamp        int64   `json:"timestamp"`
	HadError         bool    `json:"had_error"`
	Error            string  `json:"error"`
	ErrorFor         string  `json:"error_for"`
	RequestLatency   int64   `json:"request_latency_ms"`
	HeadBlockLatency int64   `json:"head_block_latency_ms"`
	PermissiveCors   bool    `json:"permissive_cors"`
	TlsVerOk         bool    `json:"tls_ver_ok"`
	TlsCipherOk      bool    `json:"tls_cipher_ok"`
	TlsNote          string  `json:"tls_note"`
	ProducerExposed  bool    `json:"producer_exposed"`
	NetExposed       bool    `json:"net_exposed"`
	FromGeo          string  `json:"from_geo"`
	Score            float32 `json:"-"`
}

// P2pResult is the output from a P2P health check
type P2pResult struct {
	Type             string `json:"type"`
	Peer             string `json:"peer"`
	TimeStamp        int64  `json:"time_stamp"`
	Took             int64  `json:"took_sec"`
	Reachable        bool   `json:"reachable"`
	Healthy          bool   `json:"healthy"`
	HeadBlockLatency int64  `json:"head_block_latency_ms"`
	ErrMsg           string `json:"err_msg"`
	FromGeo          string `json:"from_geo"`
	Score            int    `json:"-"`
}

func CombineReport(report FinalResult, files []string, path string) []FinalResult {
	combined := make([]FinalResult, len(files)+1)
	combined[len(combined)-1] = report
	sort.Strings(files)
	for i := range files {
		f, err := os.Open(path + string(os.PathSeparator) + files[i])
		if err != nil {
			log.Println(err)
			continue
		}
		j, err := ioutil.ReadAll(f)
		f.Close()
		if err != nil {
			log.Println(err)
			continue
		}
		next := FinalResult{}
		err = json.Unmarshal(j, &next)
		if err != nil {
			log.Println(err)
			continue
		}
		combined[i] = next
	}
	return combined
}

// MyGeo uses a service "address.works" to lookup the public IP being used, then uses maxmind's geolite to get a
// country and region for reporting where the check originated from. It is not smart, expects the database to be
// in the directory where the program is executing.
func MyGeo(file string) (string, error) {
	resp, err := http.Get("https://address.works/")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	ip := net.ParseIP(string(b))
	db, err := maxminddb.Open(file)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	var record struct {
		Country struct {
			ISOCode string `maxminddb:"iso_code"`
			Names   struct {
				En string `maxminddb:"en"`
			} `maxminddb:"names"`
		} `maxminddb:"country"`
		Subdivisions []struct {
			IsoCode string `maxminddb:"iso_code"`
		} `maxminddb:"subdivisions"`
	}

	err = db.Lookup(ip, &record)
	if err != nil {
		log.Fatal(err)
	}

	// If geolite cities was provided, give a little more granularity
	if record.Subdivisions != nil && len(record.Subdivisions) > 0 {
		var area string
		area = "-" + record.Subdivisions[0].IsoCode
		return record.Country.ISOCode + area, nil
	}

	return record.Country.Names.En, nil
}
