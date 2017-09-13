// replication-manager - Replication Manager Monitoring and CLI for MariaDB and MySQL
// Copyright 2017 Signal 18 SARL
// Authors: Guillaume Lefranc <guillaume@signal18.io>
//          Stephane Varoqui  <svaroqui@gmail.com>
// This source code is licensed under the GNU General Public License, version 3.
// Redistribution/Reuse of this code is permitted under the GNU v3 license, as
// an additional term, ALL code must carry the original Author(s) credit in comment form.
// See LICENSE in this directory for the integral text.

package state

import (
	"fmt"
	"sync"
	"time"
)

import "strconv"

type State struct {
	ErrType string
	ErrDesc string
	ErrFrom string
}

type StateHttp struct {
	ErrNumber string `json:"number"`
	ErrDesc   string `json:"desc"`
	ErrFrom   string `json:"from"`
}

type Map map[string]State

func NewMap() *Map {
	m := make(Map)
	return &m
}

func (m Map) Add(key string, s State) {
	_, ok := m[key]
	if !ok {
		m[key] = s

	}
}

func (m Map) Delete(key string) {
	delete(m, key)
}

func (m Map) Search(key string) bool {
	_, ok := m[key]
	if ok {
		return true
	} else {
		return false
	}

}

type StateMachine struct {
	CurState            *Map
	OldState            *Map
	discovered          bool
	lasttime            int64
	firsttime           int64
	uptime              int64
	uptimeFailable      int64
	uptimeSemisync      int64
	lastState           int64
	heartbeats          int64
	avgReplicationDelay float32
	inFailover          bool
	sync.Mutex
}

type Sla struct {
	firsttime      int64
	uptime         int64
	uptimeFailable int64
	uptimeSemisync int64
}

func (SM *StateMachine) GetSla() Sla {
	var mySla Sla
	mySla.firsttime = SM.firsttime
	mySla.uptime = SM.uptime
	mySla.uptimeFailable = SM.uptimeFailable
	mySla.uptimeSemisync = SM.uptimeSemisync
	return mySla
}

func (SM *StateMachine) SetSla(mySla Sla) {
	SM.firsttime = mySla.firsttime
	SM.uptime = mySla.uptime
	SM.uptimeFailable = mySla.uptimeFailable
	SM.uptimeSemisync = mySla.uptimeSemisync
}

func (SM *StateMachine) Init() {

	SM.CurState = NewMap()
	SM.OldState = NewMap()
	SM.discovered = false
	SM.lasttime = time.Now().Unix()
	SM.firsttime = SM.lasttime
	SM.uptime = 0
	SM.uptimeFailable = 0
	SM.uptimeSemisync = 0
	SM.lastState = 0
	SM.heartbeats = 0
}

func (SM *StateMachine) SetFailoverState() {
	SM.Lock()
	SM.inFailover = true
	SM.Unlock()
}

func (SM *StateMachine) RemoveFailoverState() {
	SM.Lock()
	SM.inFailover = false
	SM.Unlock()
}

func (SM *StateMachine) IsInFailover() bool {
	return SM.inFailover
}

func (SM *StateMachine) AddState(key string, s State) {
	SM.Lock()
	SM.CurState.Add(key, s)
	SM.Unlock()
}

func (SM *StateMachine) IsInState(key string) bool {
	SM.Lock()
	//log.Printf("%s,%s", key, SM.OldState.Search(key))
	//CurState may not be valid depending when it's call because empty at every ticker so may have not collected the state yet

	if SM.OldState.Search(key) == false {
		SM.Unlock()
		return false
	} else {
		SM.Unlock()
		return true
	}
}

func (SM *StateMachine) DeleteState(key string) {
	SM.Lock()
	SM.CurState.Delete(key)
	SM.Unlock()
}

func (SM *StateMachine) GetHeartbeats() int64 {
	return SM.heartbeats
}

func (SM *StateMachine) GetUptime() string {
	var up = strconv.FormatFloat(float64(100*float64(SM.uptime)/float64(SM.lasttime-SM.firsttime)), 'f', 5, 64)
	//fmt.Printf("INFO : Uptime %f", float64(SM.uptime)/float64(time.Now().Unix()- SM.firsttime))
	if up == "100.00000" {
		up = "99.99999"
	}
	return up
}
func (SM *StateMachine) GetUptimeSemiSync() string {

	var up = strconv.FormatFloat(float64(100*float64(SM.uptimeSemisync)/float64(SM.lasttime-SM.firsttime)), 'f', 5, 64)
	if up == "100.00000" {
		up = "99.99999"
	}
	return up
}

func (SM *StateMachine) ResetUpTime() {
	SM.lasttime = time.Now().Unix()
	SM.firsttime = SM.lasttime
	SM.uptime = 0
	SM.uptimeFailable = 0
	SM.uptimeSemisync = 0
}

func (SM *StateMachine) GetUptimeFailable() string {
	var up = strconv.FormatFloat(float64(100*float64(SM.uptimeFailable)/float64(SM.lasttime-SM.firsttime)), 'f', 5, 64)
	if up == "100.00000" {
		up = "99.99999"
	}
	return up
}

func (SM *StateMachine) IsFailable() bool {

	SM.Lock()
	for _, value := range *SM.OldState {
		if value.ErrType == "ERROR" {
			SM.Unlock()
			return false
		}
	}
	SM.discovered = true
	SM.Unlock()
	return true

}

func (SM *StateMachine) SetMasterUpAndSync(IsSemiSynced bool, IsNotDelay bool) {
	var timenow int64
	timenow = time.Now().Unix()
	if IsSemiSynced == true && SM.IsFailable() == true {
		SM.uptimeSemisync = SM.uptimeSemisync + (timenow - SM.lasttime)
	}
	if IsNotDelay == true && SM.IsFailable() == true {
		SM.uptimeFailable = SM.uptimeFailable + (timenow - SM.lasttime)
	}
	if SM.IsFailable() == true {
		SM.uptime = SM.uptime + (timenow - SM.lasttime)
	}
	SM.lasttime = timenow
	SM.heartbeats = SM.heartbeats + 1
	//fmt.Printf("INFO : is failable %b IsSemiSynced %b  IsNotDelay %b uptime %d uptimeFailable %d uptimeSemisync %d\n",SM.IsFailable(),IsSemiSynced ,IsNotDelay, SM.uptime, SM.uptimeFailable ,SM.uptimeSemisync)
}

// Clear copies the current map to argument map and clears it
func (SM *StateMachine) ClearState() {
	SM.Lock()
	SM.OldState = SM.CurState
	SM.CurState = nil
	SM.CurState = NewMap()
	SM.Unlock()
}

// CanMonitor checks if the current state contains errors and allows monitoring
func (SM *StateMachine) CanMonitor() bool {
	SM.Lock()
	for _, value := range *SM.CurState {
		if value.ErrType == "ERROR" {
			SM.Unlock()
			return false
		}
	}
	SM.discovered = true
	SM.Unlock()
	return true
}

func (SM *StateMachine) UnDiscovered() {
	SM.Lock()
	SM.discovered = false
	SM.Unlock()
}

func (SM *StateMachine) IsDiscovered() bool {
	return SM.discovered
}

func (SM *StateMachine) GetStates() []string {
	var log []string
	SM.Lock()
	//every thing in  OldState that can't be found in curstate
	for key2, value2 := range *SM.OldState {
		if SM.CurState.Search(key2) == false {
			//log = append(log, fmt.Sprintf("%-5s %s HAS BEEN FIXED, %s", value2.ErrType, key2, value2.ErrDesc))
			log = append(log, fmt.Sprintf("%s CLOSING %s", key2, value2.ErrDesc))
		}
	}

	for key, value := range *SM.CurState {
		if SM.OldState.Search(key) == false {
			//log = append(log, fmt.Sprintf("%-5s %s %s", value.ErrType, key, value.ErrDesc))
			log = append(log, fmt.Sprintf("%s %s", key, value.ErrDesc))
		}
	}
	SM.Unlock()
	return log
}

func (SM *StateMachine) GetOpenErrors() []StateHttp {
	var log []StateHttp
	SM.Lock()
	for key, value := range *SM.OldState {
		if value.ErrType == "ERROR" {
			var httplog StateHttp
			httplog.ErrDesc = value.ErrDesc
			httplog.ErrNumber = key
			httplog.ErrFrom = value.ErrFrom
			log = append(log, httplog)
		}
	}
	SM.Unlock()
	return log
}
func (SM *StateMachine) GetOpenWarnings() []StateHttp {
	var log []StateHttp
	SM.Lock()
	for key, value := range *SM.OldState {
		if value.ErrType != "ERROR" {
			var httplog StateHttp
			httplog.ErrDesc = value.ErrDesc
			httplog.ErrNumber = key
			httplog.ErrFrom = value.ErrFrom
			log = append(log, httplog)
		}
	}
	SM.Unlock()
	return log
}
