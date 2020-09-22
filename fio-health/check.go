package main

import (
	"encoding/hex"
	"fmt"
	"github.com/fioprotocol/fio-go"
	"github.com/fioprotocol/fio-go/eos"
	"github.com/fioprotocol/fio-go/eos/p2p"
	fiohealth "github.com/fioprotocol/health"
	"log"
	"math"
	"net/http"
	"strings"
	"sync"
	"time"
)

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
			if !results[i].Healthy {
				conf.p2pAlerts.HostFailed(conf.P2pNodes[i], results[i].ErrMsg)
			} else {
				conf.p2pAlerts.HostOk(conf.P2pNodes[i])
			}
			wg.Done()
		}(i)
	}
	wg.Wait()
	return results
}

func getBlockP2p(p2pnode string, geo string, conf *Config) fiohealth.P2pResult {
	started := time.Now().UTC()
	r := fiohealth.P2pResult{Type: "p2p", Peer: p2pnode, FromGeo: geo, TimeStamp: time.Now().UTC().Unix()}
	cid, err := hex.DecodeString("0x" + conf.ChainId)
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
				conf.apiAlerts.HostFailed(a, emsg, "health")
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
				conf.apiAlerts.HostFailed(a, err.Error(), "health")
				return
			}
			results[i].HeadBlockLatency = now.Sub(gi.HeadBlockTime.Time).Milliseconds()
			results[i].NodeVer = gi.ServerVersionString
			if gi.HeadBlockTime.Time.Before(time.Now().UTC().Add(-30 * time.Second)) {
				log.Println(a, "is not synced!")
				emsg := fmt.Sprintf("node head block is behind by %.2f", now.Sub(gi.HeadBlockTime.Time).Seconds())
				results[i].HadError = true
				results[i].Error = emsg
				results[i].ErrorFor = "get info"
				results[i].Score += 1
				conf.apiAlerts.HostFailed(a, emsg, "health")
				//return
			}
			if gi.ChainID.String() != conf.ChainId {
				log.Println(a, "Wrong chain!")
				results[i].HadError = true
				results[i].Error = "wrong chain"
				results[i].ErrorFor = "get info"
				results[i].Score += 5
				conf.apiAlerts.HostFailed(a, "wrong chain", "health")
			}
			_, err = api.GetBlockByNum(gi.LastIrreversibleBlockNum)
			if err != nil {
				log.Println(a, "get block", err.Error())
				results[i].HadError = true
				results[i].Error = err.Error()
				results[i].ErrorFor = "get block"
				results[i].Score += 10
				conf.apiAlerts.HostFailed(a, err.Error(), "health")
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
				conf.apiAlerts.HostFailed(a, err.Error(), "health")
			}
			if resp == nil {
				return
			}
			_ = resp.Body.Close()

			if resp.Header.Get("Access-Control-Allow-Origin") == "*" {
				results[i].PermissiveCors = true
			} else {
				results[i].Score += 1
				conf.apiAlerts.HostFailed(a, "missing permissive CORS header", "health")
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
				conf.apiAlerts.HostFailed(a, "net api is enabled", "security")
			}
			_, err = api.IsProducerPaused()
			if err == nil {
				log.Println(a, "producer api")
				results[i].ProducerExposed = true
				results[i].Score += 3
				conf.apiAlerts.HostFailed(a, "producer api is enabled", "security")
			}
			if len(notes) > 0 {
				conf.apiAlerts.HostFailed(a, strings.Join(notes, ", "), "security")
			}
		}(i, a)
	}
	wg.Wait()
	return results
}

