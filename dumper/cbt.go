package dumper

import (
	"encoding/json"
	"fmt"
)

type ConnParams struct {
	VmMoRef              string `json:"VmMoRef"`
	VsphereHostName      string `json:"VsphereHostName"`
	VsphereHostPort      int    `json:"VsphereHostPort"`
	VsphereUsername      string `json:"VsphereUsername"`
	VspherePassword      string `json:"VspherePassword"`
	VsphereThumbPrint    string `json:"VsphereThumbPrint"`
	VsphereSnapshotMoRef string `json:"VsphereSnapshotMoRef"`
}

type DiskParams struct {
	DiskPath     string `json:"diskPath"`
	DiskPathRoot string `json:"diskPathRoot"`
	ChangeId     string `json:"changeId"`
}

type ChangedArea struct {
	Start  int64 `json:"start"`
	Length int64 `json:"length"`
}

type DiskChangeInfo struct {
	StartOffset int64         `json:"startOffset"`
	Length      int64         `json:"length"`
	ChangedArea []ChangedArea `json:"changedArea"`
}

type CbtData struct {
	Conn   ConnParams     `json:"ConnParams"`
	Disk   DiskParams     `json:"DiskParams"`
	Change DiskChangeInfo `json:"DiskChangeInfo"`
}

func ParseConnParams(params string) (*ConnParams, error) {
	cp := &ConnParams{}

	err := json.Unmarshal([]byte(params), cp);
	if err != nil {
		return nil, fmt.Errorf("ParseConnParmams: %v", err)
	}
	return cp, nil
}

func ParseCbtData(conf string) (*CbtData, error) {
	cbtData := &CbtData{}

	err := json.Unmarshal([]byte(conf), cbtData);
	if err != nil {
		return nil, fmt.Errorf("ParseCbtData: %v", err)
	}
	return cbtData, nil
}
