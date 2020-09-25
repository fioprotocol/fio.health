package fiohealth

import (
	"encoding/hex"
	"github.com/fioprotocol/fio-go/eos"
	"github.com/fioprotocol/fio-go/eos/p2p"
	"log"
	"strings"
	"sync"
	"time"
)

func CheckP2p(conf *Config) (report []*P2pResult) {
	geo, err := MyGeo(conf.Geolite)
	if err != nil {
		log.Println(err)
	}
	results := make([]*P2pResult, len(conf.P2pNodes))
	wg := sync.WaitGroup{}
	wg.Add(len(conf.P2pNodes))
	for i := range conf.P2pNodes {
		go func(i int) {
			results[i] = P2pConnect(conf.P2pNodes[i], geo, conf)
			if !results[i].Healthy {
				conf.P2pAlerts.HostFailed(conf.P2pNodes[i], results[i].ErrMsg, conf.FlapSuppression)
			} else {
				conf.P2pAlerts.HostOk(conf.P2pNodes[i])
			}
			wg.Done()
		}(i)
	}
	wg.Wait()
	return results
}

func P2pConnect(p2pnode string, geo string, conf *Config) *P2pResult {
	started := time.Now().UTC()
	r := P2pResult{Type: "p2p", Peer: p2pnode, FromGeo: geo, TimeStamp: time.Now().UTC().Unix()}
	cid, err := hex.DecodeString(conf.ChainId)
	if err != nil {
		log.Fatal(err)
	}
	peer := p2p.NewOutgoingPeer(p2pnode, "healthcheck", &p2p.HandshakeInfo{
		ChainID:      cid,
		HeadBlockNum: 1,
	})
	peer.SetConnectionTimeout(10 * time.Second)
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
	return &r
}
