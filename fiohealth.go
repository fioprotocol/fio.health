package fiohealth

import (
	"encoding/json"
	"github.com/fioprotocol/fio-go"
	"github.com/fioprotocol/fio-go/eos"
	"github.com/oschwald/maxminddb-golang"
	tls "github.com/refraction-networking/utls"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
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
		f, err := os.Open(path+string(os.PathSeparator)+files[i])
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

// TestTls looks for old TLS versions and weak cipher suites, this is not the standard golang library, and it does
// not attempt to validate the certificate, knowing the settings are weak outside of having an invalid cert are
// useful findings.
func TestTls(uri string) (string, bool) {
	if !strings.HasPrefix(uri, "https") {
		return "", false
	}
	parsed, _ := url.Parse(uri)
	port := parsed.Port()
	if port == "" {
		port = "443"
	}
	hostname := parsed.Host
	addr := parsed.Host + ":" + port
	check := func(versions []uint16, suites []uint16) string {
		for _, vers := range versions {
			config := tls.Config{
				ServerName:         hostname,
				InsecureSkipVerify: true,
			}
			dialConn, err := net.DialTimeout("tcp", addr, 2*time.Second)
			if err != nil {
				log.Printf("net.DialTimeout error: %+v", err)
				return ""
			}
			uTlsConn := tls.UClient(dialConn, &config, tls.HelloCustom)

			min := vers
			if min == tls.VersionTLS13 {
				// 1.3 can't be lowest, not allowed by utls lib.
				min = tls.VersionTLS12
			}
			// most of this taken from the utls example:
			spec := tls.ClientHelloSpec{
				TLSVersMax:   vers,
				TLSVersMin:   min,
				CipherSuites: suites,
				Extensions: []tls.TLSExtension{
					&tls.SNIExtension{},
					&tls.SupportedCurvesExtension{Curves: []tls.CurveID{tls.X25519, tls.CurveP256}},
					&tls.SupportedPointsExtension{SupportedPoints: []byte{0}}, // uncompressed
					&tls.SessionTicketExtension{},
					&tls.ALPNExtension{AlpnProtocols: []string{"myFancyProtocol", "http/1.1"}},
					&tls.SignatureAlgorithmsExtension{SupportedSignatureAlgorithms: []tls.SignatureScheme{
						tls.ECDSAWithSHA1,
						tls.PKCS1WithSHA1}},
					&tls.KeyShareExtension{KeyShares: []tls.KeyShare{
						{Group: tls.CurveID(tls.GREASE_PLACEHOLDER), Data: []byte{0}},
						{Group: tls.X25519},
					}},
					&tls.PSKKeyExchangeModesExtension{Modes: []uint8{1}}, // pskModeDHE
					&tls.SupportedVersionsExtension{Versions: []uint16{vers}},
				},
				GetSessionID: nil,
			}
			err = uTlsConn.ApplyPreset(&spec)
			if err != nil {
				continue
			}
			_ = uTlsConn.SetDeadline(time.Now().Add(2 * time.Second))
			_ = uTlsConn.Handshake()
			var result string
			if uTlsConn.HandshakeState.ServerHello != nil {
				switch uTlsConn.ConnectionState().Version {
				case tls.VersionTLS10:
					result = "TLS v1.0"
				case tls.VersionTLS11:
					result = "TLS v1.1"
					//case tls.VersionTLS12:
					//	result = "TLS 1.2"
					//case tls.VersionTLS13:
					//	result = "TLS 1.3"
				}
				switch uTlsConn.ConnectionState().CipherSuite {
				case tls.TLS_RSA_WITH_RC4_128_SHA:
					result += " TLS_RSA_WITH_RC4_128_SHA"
				case tls.TLS_RSA_WITH_3DES_EDE_CBC_SHA:
					result += " TLS_RSA_WITH_3DES_EDE_CBC_SHA"
				case tls.TLS_ECDHE_ECDSA_WITH_RC4_128_SHA:
					result += " TLS_ECDHE_ECDSA_WITH_RC4_128_SHA"
				case tls.TLS_ECDHE_RSA_WITH_RC4_128_SHA:
					result += " TLS_ECDHE_RSA_WITH_RC4_128_SHA"
				case tls.TLS_ECDHE_RSA_WITH_3DES_EDE_CBC_SHA:
					result += " TLS_ECDHE_RSA_WITH_3DES_EDE_CBC_SHA"
				}
				if result == "" {
					continue
				}
				_ = uTlsConn.Close()
				return result
			}
			_ = uTlsConn.Close()
		}
		return ""
	}

	findings := check(oldVersion, weakSuites)
	if findings != "" {
		return findings, true
	}
	findings = check(oldVersion, allSuites)
	if findings != "" {
		return findings, true
	}
	findings = check(anyVersion, weakSuites)
	if findings != "" {
		return findings, true
	}

	return "", false
}

var (
	anyVersion = []uint16{tls.VersionTLS10, tls.VersionTLS11, tls.VersionTLS12, tls.VersionTLS13}
	// for now only complain about tls v1.0, even though 1.1's days are numbered.
	//oldVersion = []uint16{tls.VersionTLS10, tls.VersionTLS11}
	oldVersion = []uint16{tls.VersionTLS10}
)

var allSuites = []uint16{
	tls.TLS_RSA_WITH_RC4_128_SHA,
	tls.TLS_RSA_WITH_3DES_EDE_CBC_SHA,
	tls.TLS_RSA_WITH_AES_128_CBC_SHA,
	tls.TLS_RSA_WITH_AES_256_CBC_SHA,
	tls.TLS_RSA_WITH_AES_128_CBC_SHA256,
	tls.TLS_RSA_WITH_AES_128_GCM_SHA256,
	tls.TLS_RSA_WITH_AES_256_GCM_SHA384,
	tls.TLS_ECDHE_ECDSA_WITH_RC4_128_SHA,
	tls.TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA,
	tls.TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA,
	tls.TLS_ECDHE_RSA_WITH_RC4_128_SHA,
	tls.TLS_ECDHE_RSA_WITH_3DES_EDE_CBC_SHA,
	tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA,
	tls.TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA,
	tls.TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA256,
	tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA256,
	tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
	tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
	tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
	tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
	tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
	tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,
	tls.TLS_AES_128_GCM_SHA256,
	tls.TLS_AES_256_GCM_SHA384,
	tls.TLS_CHACHA20_POLY1305_SHA256,
}

var weakSuites = []uint16{
	tls.TLS_RSA_WITH_RC4_128_SHA,
	tls.TLS_RSA_WITH_3DES_EDE_CBC_SHA,
	tls.TLS_ECDHE_ECDSA_WITH_RC4_128_SHA,
	tls.TLS_ECDHE_RSA_WITH_RC4_128_SHA,
	tls.TLS_ECDHE_RSA_WITH_3DES_EDE_CBC_SHA,
}

// The following are used by health checks that send and receive a FIO request to confirm e2e functionality.
// If we are going to be sending a lot of requests, after a while we will run into the table query limitations, this
// gets the values directly from the table and shouldn't require a scan, maybe I should add these to the SDK?

type stat struct {
	FioRequestId uint64 `json:"fio_request_id"`
	Status       uint8  `json:"status"`
}

func HighestSent(api *fio.API, from string) (id uint64, err error) {
	hash := fio.I128Hash(from)
	gtr, err := api.GetTableRowsOrder(fio.GetTableRowsOrderRequest{
		Code:       "fio.reqobt",
		Scope:      "fio.reqobt",
		Table:      "fioreqctxts",
		LowerBound: hash,
		UpperBound: hash,
		Limit:      1,
		KeyType:    "i128",
		Index:      "3",
		JSON:       true,
		Reverse:    true,
	})
	if err != nil {
		return
	}
	status := make([]stat, 0)
	err = json.Unmarshal(gtr.Rows, &status)
	if err != nil {
		return 0, err
	}
	if len(status) == 0 {
		return 0, nil
	}
	return status[0].FioRequestId, nil
}

func IsRejected(api *fio.API, id uint64) (bool, error) {
	gtr, err := api.GetTableRows(eos.GetTableRowsRequest{
		Code:       "fio.reqobt",
		Scope:      "fio.reqobt",
		Table:      "fioreqstss",
		LowerBound: strconv.FormatUint(id, 10),
		UpperBound: strconv.FormatUint(id, 10),
		Limit:      1,
		KeyType:    "i64",
		Index:      "2",
		JSON:       true,
	})
	if err != nil {
		return false, err
	}
	status := make([]stat, 0)
	err = json.Unmarshal(gtr.Rows, &status)
	if err != nil {
		return false, err
	}
	if len(status) == 0 {
		return false, nil
	}

	if status[0].Status == 1 {
		return true, nil
	}
	return false, nil
}
