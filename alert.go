package fiohealth

import (
	"encoding/json"
	"fmt"
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

type ApiAlerts map[string]*ApiAlertState

func UnmarshalApiAlerts(b []byte) (ApiAlerts, error) {
	aa := make(ApiAlerts)
	err := json.Unmarshal(b, &aa)
	return aa, err
}

func (aa ApiAlerts) shouldAlarm(host string) bool {
	if aa[host] == nil || aa[host].sendAlarm {
		return true
	}
	if time.Now().Before(aa[host].HealthNotBefore) && aa[host].HealthAlarm {
		return false
	}
	if aa[host].SecurityAlarm && !aa[host].HealthAlarm && time.Now().Before(aa[host].SecurityNotBefore) {
		return false
	}
	return true
}

func (aa ApiAlerts) HealthOk(host string) {
	if aa[host] == nil {
		aa[host] = &ApiAlertState{}
	}
	aa[host].HealthAlarm = false
	aa[host].HealthReason = ""
}

func (aa ApiAlerts) SecurityOk(host string) {
	if aa[host] == nil {
		aa[host] = &ApiAlertState{}
	}
	aa[host].SecurityAlarm = false
	aa[host].SecurityReason = ""
}

func (aa ApiAlerts) GetAlarms() []string {
	alarms := make([]string, 0)
	for k, v := range aa {
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

func (aa ApiAlerts) HostFailed(host string, why string, healthOrSecurity string) {
	nb := time.Now().Add(time.Hour)
	if aa[host] == nil {
		aa[host] = &ApiAlertState{}
	}
	aa[host].sendAlarm = aa.shouldAlarm(host)
	switch healthOrSecurity {
	case "health":
		aa[host].HealthAlarm = true
		aa[host].HealthNotBefore = nb
		if aa[host].HealthReason != "" {
			aa[host].HealthReason = aa[host].HealthReason+"; "+why
			return
		}
		aa[host].HealthReason = why
	case "security":
		aa[host].SecurityAlarm = true
		aa[host].SecurityNotBefore = nb
		if aa[host].SecurityReason != "" {
			aa[host].SecurityReason = aa[host].SecurityReason+"; "+why
			return
		}
		aa[host].SecurityReason = why
	}
	return
}

func (aa ApiAlerts) ToJson() ([]byte, error) {
	return json.MarshalIndent(aa, "", "  ")
}

type P2pAlertState struct {
	sendAlarm bool

	Alarm     bool      `json:"alarm"`
	Reason    string    `json:"reason"`
	NotBefore time.Time `json:"not_before"`
}

type P2pAlerts map[string]*P2pAlertState

func UnmarshalP2pAlerts(b []byte) (P2pAlerts, error) {
	pa := make(P2pAlerts)
	err := json.Unmarshal(b, &pa)
	return pa, err
}

func (pa P2pAlerts) shouldAlarm(host string) bool {
	if pa[host] == nil {
		return true
	}
	if pa[host].Alarm || time.Now().After(pa[host].NotBefore) {
		return false
	}
	return true
}

func (pa P2pAlerts) HostOk(host string) {
	if pa[host] == nil {
		pa[host] = &P2pAlertState{}
	}
	pa[host].Alarm = false
	pa[host].Reason = ""
}

func (pa P2pAlerts) HostFailed(host string, reason string) (shouldAlert bool) {
	if pa[host] == nil {
		pa[host] = &P2pAlertState{}
	}
	pa[host].sendAlarm = pa.shouldAlarm(host)
	pa[host].Alarm = true
	pa[host].NotBefore = time.Now().Add(time.Hour)
	if pa[host].Reason != "" {
		pa[host].Reason = pa[host].Reason+"; " +reason
		return
	}
	pa[host].Reason = reason
	return
}

func (pa P2pAlerts) GetAlarms() []string {
	alarms := make([]string, 0)
	for k, v := range pa {
		if v.sendAlarm {
			alarms = append(alarms, fmt.Sprintf("P2P health warning: %s - %s", k, v.Reason))
		}
	}
	return alarms
}

func (pa P2pAlerts) ToJson() ([]byte, error) {
	return json.MarshalIndent(pa, "", "  ")
}


