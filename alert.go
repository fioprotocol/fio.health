package fiohealth

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// ApiAlertState holds the alarm status for a node
type ApiAlertState struct {
	sendAlarm bool

	HealthAlarm     bool      `json:"health_alarm"`
	HealthReason    string    `json:"health_reason"`
	HealthNotBefore time.Time `json:"health_not_before"`

	SecurityAlarm     bool      `json:"security_alarm"`
	SecurityReason    string    `json:"security_reason"`
	SecurityNotBefore time.Time `json:"security_not_before"`
}

// ApiAlerts contains all api alarms, is marshalled and stored to reduce alarm fatigue
type ApiAlerts struct {
	State map[string]*ApiAlertState `json:"state"`
	sync.Mutex
}

// UnmarshalApiAlerts converts from json
func UnmarshalApiAlerts(b []byte) (ApiAlerts, error) {
	aa := ApiAlerts{}
	aa.State = make(map[string]*ApiAlertState)
	err := json.Unmarshal(b, &aa)
	return aa, err
}

// shouldAlarm determines if a new alarm should be generated, should be called *before* updating state.
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

// HealthOk resets the health state for an endpoint
func (aa *ApiAlerts) HealthOk(host string) {
	aa.Lock()
	defer aa.Unlock()
	if aa.State[host] == nil {
		aa.State[host] = &ApiAlertState{}
	}
	aa.State[host].HealthAlarm = false
	aa.State[host].HealthReason = ""
}

// SecurityOk resets the security state for an endpoint
func (aa *ApiAlerts) SecurityOk(host string) {
	aa.Lock()
	defer aa.Unlock()
	if aa.State[host] == nil {
		aa.State[host] = &ApiAlertState{}
	}
	aa.State[host].SecurityAlarm = false
	aa.State[host].SecurityReason = ""
}

// GetAlarms provides a list of alarms that need to be sent to telegram
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

// HostFailed saves a failure into alarm state, calls shouldAlarm to mark as needing an alert
func (aa *ApiAlerts) HostFailed(host string, why string, healthOrSecurity string, suppress int) {
	aa.Lock()
	defer aa.Unlock()
	nb := time.Now().Add(time.Duration(suppress) * time.Hour)
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

// ToJson marshals
func (aa *ApiAlerts) ToJson() ([]byte, error) {
	aa.Lock()
	defer aa.Unlock()
	return json.MarshalIndent(aa, "", "  ")
}

// P2pAlertState represents the alarm state for a p2p node
type P2pAlertState struct {
	sendAlarm bool

	Alarm     bool      `json:"alarm"`
	Reason    string    `json:"reason"`
	NotBefore time.Time `json:"not_before"`
}

// P2pAlerts holds all the p2p alarms, and is stored each run to reduce alarm fatigue
type P2pAlerts struct {
	State map[string]*P2pAlertState
	sync.Mutex
}

// UnmarshalP2pAlerts converts from json
func UnmarshalP2pAlerts(b []byte) (P2pAlerts, error) {
	pa := P2pAlerts{}
	err := json.Unmarshal(b, &pa)
	if pa.State == nil {
		pa.State = make(map[string]*P2pAlertState)
	}
	return pa, err
}

// shouldAlarm determines if a new alarm should be generated, should be called *before* updating state.
func (pa *P2pAlerts) shouldAlarm(host string) bool {
	if pa.State[host] == nil {
		return true
	}
	if time.Now().Before(pa.State[host].NotBefore) {
		return false
	}
	return true
}

// HostOk resets a p2p node to healthy state
func (pa *P2pAlerts) HostOk(host string) {
	pa.Lock()
	defer pa.Unlock()
	if pa.State[host] == nil {
		pa.State[host] = &P2pAlertState{}
	}
	pa.State[host].Alarm = false
	pa.State[host].Reason = ""
}

// HostFailed stores a test failure
func (pa *P2pAlerts) HostFailed(host string, reason string, suppression int) (shouldAlert bool) {
	pa.Lock()
	defer pa.Unlock()
	if pa.State[host] == nil {
		pa.State[host] = &P2pAlertState{}
	}
	pa.State[host].sendAlarm = pa.shouldAlarm(host)
	pa.State[host].Alarm = true
	pa.State[host].NotBefore = time.Now().Add(time.Duration(suppression) * time.Hour)
	if pa.State[host].Reason != "" {
		pa.State[host].Reason = pa.State[host].Reason + "; " + reason
		return
	}
	pa.State[host].Reason = reason
	return
}

// GetAlarms returns all of the new failures that need alerting
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

// ToJson marshals the alerts
func (pa *P2pAlerts) ToJson() ([]byte, error) {
	pa.Lock()
	defer pa.Unlock()
	return json.MarshalIndent(pa, "", "  ")
}
