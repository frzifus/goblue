package goblue

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type HttpClient interface {
	Do(req *http.Request) (*http.Response, error)
}

type VehicleStatus struct {
	updatedAt    time.Time
	doorIsLocked bool
	isCharging   bool
	batterySoc   int
	rangeLeft    int
	targetSocAC  int
	targetSocDC  int
	plugState    int // TODO: 0 == unplugged
}

func (v *VehicleStatus) UpdatedAt() time.Time {
	return v.updatedAt
}

func (v *VehicleStatus) DoorIsLocked() bool {
	return v.doorIsLocked
}

func (v *VehicleStatus) IsCharging() bool {
	return v.isCharging
}

func (v *VehicleStatus) SoC() int {
	return v.batterySoc
}

func (v *VehicleStatus) RangeLeft() int {
	return v.rangeLeft
}

func (v *VehicleStatus) MaxRange() int {
	if v.batterySoc == 0 {
		return 0
	}
	return v.rangeLeft / v.batterySoc * 100
}

func (v *VehicleStatus) TargetSocDC() int {
	return v.targetSocDC
}

func (v *VehicleStatus) TargetSocAC() int {
	return v.targetSocAC
}

type StartOptions struct{} // ClimateOptions || StartOptions

type VehicleOption func(*Vehicle)

func WithVehicleClient(h HttpClient) VehicleOption {
	return func(v *Vehicle) {
		v.http = h
	}
}
func WithVehicleAuth(a auth) VehicleOption {
	return func(v *Vehicle) {
		v.auth = a
	}
}
func WithVehicleEndpoints(e endpoints) VehicleOption {
	return func(v *Vehicle) {
		v.endpoints = e
	}
}

func NewVehicle(
	id, vin, name, vtype string,
	b Brand,
	opts ...VehicleOption,
) *Vehicle {
	v := &Vehicle{id: id, vin: vin, name: name, vtype: vtype, brand: b}
	for _, o := range opts {
		o(v)
	}
	return v
}

type Vehicle struct {
	id    string
	vin   string
	name  string
	vtype string
	brand Brand

	http      HttpClient
	auth      auth
	endpoints endpoints
}

func (v *Vehicle) VIN() string  { return v.vin }
func (v *Vehicle) ID() string   { return v.id }
func (v *Vehicle) Name() string { return v.name }
func (v *Vehicle) Type() string { return v.vtype }
func (v *Vehicle) Brand() Brand { return v.brand }

func (v *Vehicle) Unlock() error               { return ErrNotImplemented }
func (v *Vehicle) Lock() error                 { return ErrNotImplemented }
func (v *Vehicle) Start(...StartOptions) error { return ErrNotImplemented }
func (v *Vehicle) Stop() error                 { return ErrNotImplemented }
func (v *Vehicle) Location() (string, error)   { return "", ErrNotImplemented }
func (v *Vehicle) Odometer() (string, error)   { return "", ErrNotImplemented }
func (v *Vehicle) Status() (*VehicleStatus, error) {
	if v.auth.AccessToken == "" {
		return nil, ErrNotAuthenticated
	}

	stamp, err := GetStampFromList(v.brand)
	if err != nil {
		return nil, err
	}

	headers := map[string]string{
		"Authorization":       v.auth.AccessToken,
		"ccsp-device-id":      v.auth.CCSPDeviceID,
		"ccsp-application-id": v.auth.CCSPApplicationID,
		"offset":              "1",
		"User-Agent":          v.auth.UserAgent,
		"Stamp":               stamp,
	}

	uri := fmt.Sprintf(v.auth.URI+v.endpoints.Status, v.id)
	req, err := newHttpRequest(http.MethodGet, uri, nil, headers)
	if err != nil {
		return nil, err
	}

	resp, err := v.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized ||
		resp.StatusCode == http.StatusForbidden {
		return nil, ErrNotAuthenticated
	}

	msg := vehicleStatusResponse{}
	var buf bytes.Buffer
	buf.ReadFrom(resp.Body)
	if err := json.Unmarshal(buf.Bytes(), &msg); err != nil {
		return nil, err
	}
	if msg.Retcode != apiCodeOk {
		return nil, ErrNotAuthenticated
	}

	var rangebyfuel int
	if len(msg.Resmsg.Evstatus.Drvdistance) > 0 {
		rangebyfuel = msg.Resmsg.Evstatus.Drvdistance[0].Rangebyfuel.Evmoderange.Value
	}

	var acTarget, dcTarget int
	for _, t := range msg.Resmsg.Evstatus.Reservchargeinfos.Targetsoclist {
		switch t.Plugtype {
		case PlugTypeAC:
			acTarget = t.Targetsoclevel
		case PlugTypeDC:
			dcTarget = t.Targetsoclevel
		}
	}

	return &VehicleStatus{
		updatedAt:    time.Now(), // TODO: grep from response
		doorIsLocked: msg.Resmsg.Doorlock,
		isCharging:   msg.Resmsg.Evstatus.Batterycharge,
		batterySoc:   msg.Resmsg.Evstatus.Batterystatus,
		plugState:    msg.Resmsg.Evstatus.Batteryplugin,
		rangeLeft:    rangebyfuel,
		targetSocAC:  acTarget,
		targetSocDC:  dcTarget,
	}, nil
}

type PlugType int

const (
	PlugTypeAC PlugType = iota
	PlugTypeDC
)

// thx to https://mholt.github.io/json-to-go/ :)
type vehicleStatusResponse struct {
	Retcode string `json:"retCode"`
	Rescode string `json:"resCode"`
	Resmsg  struct {
		Airctrlon bool `json:"airCtrlOn"`
		Engine    bool `json:"engine"`
		Doorlock  bool `json:"doorLock"`
		Dooropen  struct {
			Frontleft  int `json:"frontLeft"`
			Frontright int `json:"frontRight"`
			Backleft   int `json:"backLeft"`
			Backright  int `json:"backRight"`
		} `json:"doorOpen"`
		Trunkopen bool `json:"trunkOpen"`
		Airtemp   struct {
			Value        string `json:"value"`
			Unit         int    `json:"unit"`
			Hvactemptype int    `json:"hvacTempType"`
		} `json:"airTemp"`
		Defrost  bool `json:"defrost"`
		Acc      bool `json:"acc"`
		Evstatus struct {
			Batterycharge bool `json:"batteryCharge"`
			Batterystatus int  `json:"batteryStatus"`
			Batteryplugin int  `json:"batteryPlugin"`
			Remaintime2   struct {
				Etc1 struct {
					Value int `json:"value"`
					Unit  int `json:"unit"`
				} `json:"etc1"`
				Etc2 struct {
					Value int `json:"value"`
					Unit  int `json:"unit"`
				} `json:"etc2"`
				Etc3 struct {
					Value int `json:"value"`
					Unit  int `json:"unit"`
				} `json:"etc3"`
				Atc struct {
					Value int `json:"value"`
					Unit  int `json:"unit"`
				} `json:"atc"`
			} `json:"remainTime2"`
			Drvdistance []struct {
				Rangebyfuel struct {
					Evmoderange struct {
						Value int `json:"value"`
						Unit  int `json:"unit"`
					} `json:"evModeRange"`
					Totalavailablerange struct {
						Value int `json:"value"`
						Unit  int `json:"unit"`
					} `json:"totalAvailableRange"`
				} `json:"rangeByFuel"`
				Type int `json:"type"`
			} `json:"drvDistance"`
			Reservchargeinfos struct {
				Reservchargeinfo struct {
					Reservchargeinfodetail struct {
						Reservinfo struct {
							Day  []int `json:"day"`
							Time struct {
								Time        string `json:"time"`
								Timesection int    `json:"timeSection"`
							} `json:"time"`
						} `json:"reservInfo"`
						Reservchargeset bool `json:"reservChargeSet"`
						Reservfatcset   struct {
							Defrost bool `json:"defrost"`
							Airtemp struct {
								Value        string `json:"value"`
								Unit         int    `json:"unit"`
								Hvactemptype int    `json:"hvacTempType"`
							} `json:"airTemp"`
							Airctrl  int `json:"airCtrl"`
							Heating1 int `json:"heating1"`
						} `json:"reservFatcSet"`
					} `json:"reservChargeInfoDetail"`
				} `json:"reservChargeInfo"`
				Offpeakpowerinfo struct {
					Offpeakpowertime1 struct {
						Starttime struct {
							Time        string `json:"time"`
							Timesection int    `json:"timeSection"`
						} `json:"starttime"`
						Endtime struct {
							Time        string `json:"time"`
							Timesection int    `json:"timeSection"`
						} `json:"endtime"`
					} `json:"offPeakPowerTime1"`
					Offpeakpowerflag int `json:"offPeakPowerFlag"`
				} `json:"offpeakPowerInfo"`
				Reservechargeinfo2 struct {
					Reservchargeinfodetail struct {
						Reservinfo struct {
							Day  []int `json:"day"`
							Time struct {
								Time        string `json:"time"`
								Timesection int    `json:"timeSection"`
							} `json:"time"`
						} `json:"reservInfo"`
						Reservchargeset bool `json:"reservChargeSet"`
						Reservfatcset   struct {
							Defrost bool `json:"defrost"`
							Airtemp struct {
								Value        string `json:"value"`
								Unit         int    `json:"unit"`
								Hvactemptype int    `json:"hvacTempType"`
							} `json:"airTemp"`
							Airctrl  int `json:"airCtrl"`
							Heating1 int `json:"heating1"`
						} `json:"reservFatcSet"`
					} `json:"reservChargeInfoDetail"`
				} `json:"reserveChargeInfo2"`
				Reservflag int `json:"reservFlag"`
				Ect        struct {
					Start struct {
						Day  int `json:"day"`
						Time struct {
							Time        string `json:"time"`
							Timesection int    `json:"timeSection"`
						} `json:"time"`
					} `json:"start"`
					End struct {
						Day  int `json:"day"`
						Time struct {
							Time        string `json:"time"`
							Timesection int    `json:"timeSection"`
						} `json:"time"`
					} `json:"end"`
				} `json:"ect"`
				Targetsoclist []struct {
					Targetsoclevel int      `json:"targetSOClevel"`
					Plugtype       PlugType `json:"plugType"`
				} `json:"targetSOClist"`
			} `json:"reservChargeInfos"`
		} `json:"evStatus"`
		Ign3               bool `json:"ign3"`
		Hoodopen           bool `json:"hoodOpen"`
		Transcond          bool `json:"transCond"`
		Steerwheelheat     int  `json:"steerWheelHeat"`
		Sidebackwindowheat int  `json:"sideBackWindowHeat"`
		Tirepressurelamp   struct {
			Tirepressurelampall int `json:"tirePressureLampAll"`
			Tirepressurelampfl  int `json:"tirePressureLampFL"`
			Tirepressurelampfr  int `json:"tirePressureLampFR"`
			Tirepressurelamprl  int `json:"tirePressureLampRL"`
			Tirepressurelamprr  int `json:"tirePressureLampRR"`
		} `json:"tirePressureLamp"`
		Battery struct {
			Batsoc   int `json:"batSoc"`
			Batstate int `json:"batState"`
		} `json:"battery"`
		Time string `json:"time"`
	} `json:"resMsg"`
	Msgid string `json:"msgId"`
}
