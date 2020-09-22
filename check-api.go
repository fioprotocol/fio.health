package fiohealth

import (
	"crypto/tls"
	"fmt"
	"github.com/fioprotocol/fio-go"
	"log"
	"math"
	"net/http"
	"strings"
	"sync"
	"time"
)

// CheckApis runs the health checks for the API nodes, each node is tested concurrently, timeouts are set for a short
// interval. Checks: connection latency, head block lag, chainId is correct, logs (and checks) for expected version,
// if CORS is permissive, that TLS is enabled, checks for weak TLS ciphers and deprecated version, ensures that
// the negotiated protocol is TLSv1.2 or higher, alarms if certificate expires within 30 days, and ensures that neither
// the producer and network API is exposed.
func CheckApis(conf *Config) (report []Result) {
	myIpAddr, err := MyGeo(conf.Geolite)
	if err != nil {
		log.Fatal(err)
	}
	wg := sync.WaitGroup{}
	wg.Add(len(conf.ApiNodes))
	results := make([]Result, len(conf.ApiNodes))
	for i, a := range conf.ApiNodes {
		go func(i int, a string) {
			defer wg.Done()
			results[i] = Result{
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
				conf.ApiAlerts.HostFailed(a, emsg, "health")
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
				conf.ApiAlerts.HostFailed(a, err.Error(), "health")
				return
			}
			results[i].HeadBlockLatency = now.Sub(gi.HeadBlockTime.Time).Milliseconds()
			results[i].NodeVer = gi.ServerVersionString
			if !strings.HasPrefix(results[i].NodeVer, conf.ExpectedVersionPrefix) {
				results[i].WrongVersion = true
			}
			if gi.HeadBlockTime.Time.Before(time.Now().UTC().Add(-30 * time.Second)) {
				log.Println(a, "is not synced!")
				emsg := fmt.Sprintf("node head block is behind by %.2f", now.Sub(gi.HeadBlockTime.Time).Seconds())
				results[i].HadError = true
				results[i].Error = emsg
				results[i].ErrorFor = "get info"
				results[i].Score += 1
				conf.ApiAlerts.HostFailed(a, emsg, "health")
			}
			if gi.ChainID.String() != conf.ChainId {
				log.Println(a, "Wrong chain!")
				results[i].HadError = true
				results[i].Error = "wrong chain"
				results[i].ErrorFor = "get info"
				results[i].Score += 5
				conf.ApiAlerts.HostFailed(a, "wrong chain", "health")
			}
			_, err = api.GetBlockByNum(gi.LastIrreversibleBlockNum)
			if err != nil {
				log.Println(a, "get block", err.Error())
				results[i].HadError = true
				results[i].Error = err.Error()
				results[i].ErrorFor = "get block"
				results[i].Score += 10
				conf.ApiAlerts.HostFailed(a, err.Error(), "health")
				return
			}

			notes := make([]string, 0)

			if finding, found := TestTls(api.BaseURL); found {
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
				conf.ApiAlerts.HostFailed(a, err.Error(), "health")
			}
			if resp == nil {
				return
			}
			_ = resp.Body.Close()

			if resp.Header.Get("Access-Control-Allow-Origin") == "*" {
				results[i].PermissiveCors = true
			} else {
				results[i].Score += 1
				conf.ApiAlerts.HostFailed(a, "missing permissive CORS header", "health")
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
						appendIf(fmt.Sprintf("cert expires in %d days", int64(math.Round(expires))))
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
				conf.ApiAlerts.HostFailed(a, "net api is enabled", "security")
			}
			_, err = api.IsProducerPaused()
			if err == nil {
				log.Println(a, "producer api")
				results[i].ProducerExposed = true
				results[i].Score += 3
				conf.ApiAlerts.HostFailed(a, "producer api is enabled", "security")
			}
			if len(notes) > 0 {
				conf.ApiAlerts.HostFailed(a, strings.Join(notes, ", "), "security")
			} else {
				conf.ApiAlerts.HealthOk(a)
			}
		}(i, a)
	}
	wg.Wait()
	return results
}
