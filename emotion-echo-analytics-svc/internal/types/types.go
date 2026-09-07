
package types

type HealthResp struct {
	Status  string `json:"status"`
	Time    int64  `json:"time"`
	Service string `json:"service"`
	Version string `json:"version"`
	DbOK    bool   `json:"dbOk"`
}
