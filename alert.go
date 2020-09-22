package fiohealth

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

type ApiAlertState struct {
	sendAlarm bool

	HealthAlarm     bool      `json:"health_alarm"`
	HealthReason    string    `json:"health_reason"`
	HealthNotBefore time.Time `json:"health_not_before"`

	SecurityAlarm     bool      `json:"security_alarm"`
	SecurityReason    string    `json:"security_reason"`
	SecurityNotBefore time.Time `json:"security_not_before"`
}

type ApiAlerts struct {
	State map[string]*ApiAlertState `json:"state"`
	sync.Mutex
}

func UnmarshalApiAlerts(b []byte) (ApiAlerts, error) {
	aa := ApiAlerts{}
	aa.State = make(map[string]*ApiAlertState)
	err := json.Unmarshal(b, &aa)
	return aa, err
}

func (aa *ApiAlerts) shouldAlarm(host string) bool {
	if aa.State[host] == nil || aa.State[host].sendAlarm {
		return true
	}
	if time.Now().Before(aa.State[host].HealthNotBefore) && aa.State[host].HealthAlarm {
		return false
	}
	if aa.State[host].SecurityAlarm && !aa.State[host].HealthAlarm && time.Now().Before(aa.State[host].SecurityNotBefore) {
		return false
	}
	return true
}

func (aa *ApiAlerts) HealthOk(host string) {
	aa.Lock()
	defer aa.Unlock()
	if aa.State[host] == nil {
		aa.State[host] = &ApiAlertState{}
	}
	aa.State[host].HealthAlarm = false
	aa.State[host].HealthReason = ""
}

func (aa *ApiAlerts) SecurityOk(host string) {
	aa.Lock()
	defer aa.Unlock()
	if aa.State[host] == nil {
		aa.State[host] = &ApiAlertState{}
	}
	aa.State[host].SecurityAlarm = false
	aa.State[host].SecurityReason = ""
}

func (aa *ApiAlerts) GetAlarms() []string {
	aa.Lock()
	defer aa.Unlock()
	alarms := make([]string, 0)
	for k, v := range aa.State {
		if v.sendAlarm {
			if v.HealthAlarm {
				alarms = append(alarms, fmt.Sprintf("Health warning: %s - %s", k, v.HealthReason))
			}
			if v.SecurityAlarm {
				alarms = append(alarms, fmt.Sprintf("Security warning: %s - %s", k, v.SecurityReason))
			}
		}
	}
	return alarms
}

func (aa *ApiAlerts) HostFailed(host string, why string, healthOrSecurity string) {
	aa.Lock()
	defer aa.Unlock()
	nb := time.Now().Add(time.Hour)
	if aa.State[host] == nil {
		aa.State[host] = &ApiAlertState{}
	}
	aa.State[host].sendAlarm = aa.shouldAlarm(host)
	// alert repeatedly on impending TLS expiration
	if why == "cert expires in 1 days" {
		aa.State[host].sendAlarm = true
	}
	switch healthOrSecurity {
	case "health":
		aa.State[host].HealthAlarm = true
		aa.State[host].HealthNotBefore = nb
		if aa.State[host].HealthReason != "" {
			aa.State[host].HealthReason = aa.State[host].HealthReason + "; " + why
			return
		}
		aa.State[host].HealthReason = why
	case "security":
		aa.State[host].SecurityAlarm = true
		aa.State[host].SecurityNotBefore = nb
		if aa.State[host].SecurityReason != "" {
			aa.State[host].SecurityReason = aa.State[host].SecurityReason + "; " + why
			return
		}
		aa.State[host].SecurityReason = why
	}
	return
}

func (aa *ApiAlerts) ToJson() ([]byte, error) {
	aa.Lock()
	defer aa.Unlock()
	return json.MarshalIndent(aa, "", "  ")
}

type P2pAlertState struct {
	sendAlarm bool

	Alarm     bool      `json:"alarm"`
	Reason    string    `json:"reason"`
	NotBefore time.Time `json:"not_before"`
}

type P2pAlerts struct {
	State map[string]*P2pAlertState
	sync.Mutex
}

func UnmarshalP2pAlerts(b []byte) (P2pAlerts, error) {
	pa := P2pAlerts{}
	err := json.Unmarshal(b, &pa)
	if pa.State == nil {
		pa.State = make(map[string]*P2pAlertState)
	}
	return pa, err
}

func (pa *P2pAlerts) shouldAlarm(host string) bool {
	if pa.State[host] == nil {
		return true
	}
	if time.Now().Before(pa.State[host].NotBefore) {
		return false
	}
	return true
}

func (pa *P2pAlerts) HostOk(host string) {
	pa.Lock()
	defer pa.Unlock()
	if pa.State[host] == nil {
		pa.State[host] = &P2pAlertState{}
	}
	pa.State[host].Alarm = false
	pa.State[host].Reason = ""
}

func (pa *P2pAlerts) HostFailed(host string, reason string) (shouldAlert bool) {
	pa.Lock()
	defer pa.Unlock()
	if pa.State[host] == nil {
		pa.State[host] = &P2pAlertState{}
	}
	pa.State[host].sendAlarm = pa.shouldAlarm(host)
	pa.State[host].Alarm = true
	pa.State[host].NotBefore = time.Now().Add(time.Hour)
	if pa.State[host].Reason != "" {
		pa.State[host].Reason = pa.State[host].Reason + "; " + reason
		return
	}
	pa.State[host].Reason = reason
	return
}

func (pa *P2pAlerts) GetAlarms() []string {
	pa.Lock()
	defer pa.Unlock()
	alarms := make([]string, 0)
	for k, v := range pa.State {
		if v.sendAlarm {
			alarms = append(alarms, fmt.Sprintf("P2P health warning: %s - %s", k, v.Reason))
		}
	}
	return alarms
}

func (pa *P2pAlerts) ToJson() ([]byte, error) {
	pa.Lock()
	defer pa.Unlock()
	return json.MarshalIndent(pa, "", "  ")
}
