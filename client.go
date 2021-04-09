package goblue

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/net/publicsuffix"
)

const (
	defaultUserAgent = "okhttp/3.10.0"

	apiCodeOk   = "S"
	apiCodeFail = "F"
)

// auth bundles miscellaneous authorization data
type auth struct {
	URI               string
	TokenAuth         string
	CCSPServiceID     string
	CCSPApplicationID string
	CCSPDeviceID      string
	AccessToken       string
	UserAgent         string
}

type ClientOptions func(*Client) error

func WithTransport(rt http.RoundTripper) ClientOptions {
	return func(c *Client) error {
		c.http.Transport = rt
		return nil
	}
}

func WithTimeout(d time.Duration) ClientOptions {
	return func(c *Client) error {
		c.http.Timeout = d
		return nil
	}
}

func NewClient(cfg Config, opts ...ClientOptions) (*Client, error) {
	cl := &Client{
		http: &http.Client{
			Timeout:   45 * time.Minute,
			Transport: http.DefaultTransport,
		},
		endpoints: defaultEndpoints(),
		cfg:       cfg,
		auth: auth{
			UserAgent: defaultUserAgent,
		},
	}

	switch cfg.Brand {
	case BrandHyundai:
		cl.auth.URI = "https://prd.eu-ccapi.hyundai.com:8080"
		cl.auth.CCSPServiceID = "6d477c38-3ca4-4cf3-9557-2a1929a94654"
		cl.auth.CCSPApplicationID = "99cfff84-f4e2-4be8-a5ed-e5b755eb6581"
		cl.auth.TokenAuth = "NmQ0NzdjMzgtM2NhNC00Y2YzLTk1NTctMmExOTI5YTk0NjU0OktVeTQ5WHhQekxwTHVvSzB4aEJDNzdXNlZYaG10UVI5aVFobUlGampvWTRJcHhzVg=="
	case BrandKia:
		cl.auth.URI = "https://prd.eu-ccapi.kia.com:8080"
		cl.auth.CCSPServiceID = "fdc85c00-0a2f-4c64-bcb4-2cfb1500730a"
		cl.auth.CCSPApplicationID = "693a33fa-c117-43f2-ae3b-61a02d24f417"
		cl.auth.TokenAuth = "ZmRjODVjMDAtMGEyZi00YzY0LWJjYjQtMmNmYjE1MDA3MzBhOnNlY3JldA=="
	default:
		return nil, ErrUnknownBrand
	}

	for _, o := range opts {
		if err := o(cl); err != nil {
			return nil, err
		}
	}

	return cl, nil
}

type Client struct {
	http      *http.Client
	auth      auth
	endpoints endpoints
	cfg       Config
}

func (c *Client) Vehicles() ([]*Vehicle, error) {
	if c.auth.AccessToken == "" {
		return nil, ErrNotAuthenticated
	}

	stamp, err := GetStampFromList(c.cfg.Brand)
	if err != nil {
		return nil, err
	}

	headers := map[string]string{
		"Authorization":       c.auth.AccessToken,
		"ccsp-device-id":      c.auth.CCSPDeviceID,
		"ccsp-application-id": c.auth.CCSPApplicationID,
		"offset":              "1",
		"User-Agent":          c.auth.UserAgent,
		"Stamp":               stamp,
	}

	uri := c.auth.URI + c.endpoints.Vehicles
	req, err := newHttpRequest(http.MethodGet, uri, nil, headers)
	if err != nil {
		return nil, err
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	msg := struct {
		Retcode string `json:"retCode"`
		Rescode string `json:"resCode"`
		Resmsg  struct {
			Vehicles []struct {
				ID         string `json:"vehicleId"`
				Vin        string `json:"vin"`
				Name       string `json:"vehicleName"`
				Type       string `json:"type"`
				Nickname   string `json:"nickname"`
				Master     bool   `json:"master"`
				Carshare   int    `json:"carShare"`
				Regdate    string `json:"regDate"`
				Detailinfo struct {
					Salecarmdlcd   string `json:"saleCarmdlCd"`
					Bodytype       string `json:"bodyType"`
					Incolor        string `json:"inColor"`
					Outcolor       string `json:"outColor"`
					Salecarmdlennm string `json:"saleCarmdlEnNm"`
				} `json:"detailInfo"`
			} `json:"vehicles"`
		} `json:"resMsg"`
		Msgid string `json:"msgId"`
	}{}

	var buf bytes.Buffer
	buf.ReadFrom(resp.Body)
	if err := json.Unmarshal(buf.Bytes(), &msg); err != nil {
		return nil, err
	}
	if msg.Retcode != apiCodeOk {
		return nil, ErrNotAuthenticated
	}

	count := len(msg.Resmsg.Vehicles)
	if count == 0 {
		return nil, ErrNoVehicleFound
	}

	vehicles := make([]*Vehicle, count)
	for i, v := range msg.Resmsg.Vehicles {
		vehicles[i] = NewVehicle(
			v.ID, v.Vin, v.Name, v.Type,
			c.cfg.Brand,
			WithVehicleClient(c.http),
			WithVehicleAuth(c.auth),
			WithVehicleEndpoints(c.endpoints),
		)
	}

	return vehicles, nil
}

func (c *Client) Authenticate() error {
	c.resetCookies()

	deviceID, err := c.requestDeviceID()
	if err != nil {
		return err
	}
	c.auth.CCSPDeviceID = deviceID

	if err := c.setCookiesAndVerify(); err != nil {
		return err
	}

	const langEnglish = "en"
	if err := c.setLanguage(langEnglish); err != nil {
		return err
	}

	var accCode string
	if accCode, err = c.login(); err != nil {
		return err
	}

	token, err := c.requestAccessToken(accCode)
	if err != nil {
		return err
	}
	c.auth.AccessToken = token

	return nil
}

func (c *Client) requestDeviceID() (string, error) {
	uniID, err := uuid.NewUUID()
	if err != nil {
		return "", err
	}

	data := map[string]interface{}{
		"pushRegId": "1",
		"pushType":  "GCM",
		"uuid":      uniID.String(),
	}

	stamp, err := GetStampFromList(c.cfg.Brand)
	if err != nil {
		return "", err
	}

	headers := map[string]string{
		"ccsp-service-id": c.auth.CCSPServiceID,
		"Content-type":    "application/json;charset=UTF-8",
		"User-Agent":      c.auth.UserAgent,
		"Stamp":           stamp,
	}

	body, err := json.Marshal(data)
	if err != nil {
		return "", err
	}

	uri := c.auth.URI + c.endpoints.DeviceID
	req, err := newHttpRequest(http.MethodPost, uri, bytes.NewReader(body), headers)
	if err != nil {
		return "", err
	}
	r, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer r.Body.Close()

	if r.StatusCode != http.StatusOK {
		return "", ErrNotAuthenticated
	}

	txt, err := io.ReadAll(r.Body)
	if err != nil {
		return "", err
	}

	resp := struct {
		Retcode string `json:"retCode"`
		Resmsg  struct {
			Deviceid string `json:"deviceId"`
		} `json:"resMsg"`
	}{}
	if err := json.Unmarshal(txt, &resp); err != nil {
		return "", err
	}

	if resp.Retcode == apiCodeFail {
		return "", ErrNotAuthenticated
	}

	return resp.Resmsg.Deviceid, nil

}

func (c *Client) resetCookies() {
	c.http.Jar = nil
}

func (c *Client) setCookiesAndVerify() error {
	jar, err := cookiejar.New(&cookiejar.Options{PublicSuffixList: publicsuffix.List})
	if err != nil {
		return err
	}
	c.http.Jar = jar

	uri := fmt.Sprintf(
		"%s/api/v1/user/oauth2/authorize?response_type=code&state=test&client_id=%s&redirect_uri=%s/api/v1/user/oauth2/redirect",
		c.auth.URI,
		c.auth.CCSPServiceID,
		c.auth.URI,
	)

	var resp *http.Response
	if resp, err = c.http.Get(uri); err != nil {
		return err
	}
	resp.Body.Close()

	return err
}

func (c *Client) setLanguage(lang string) error {
	data := map[string]interface{}{
		"lang": lang,
	}

	body, err := json.Marshal(data)
	if err != nil {
		return err
	}

	uri := c.auth.URI + c.endpoints.Lang
	req, err := newHttpRequest(http.MethodPost, uri, bytes.NewReader(body), JSONEncoding)
	if err != nil {
		return err
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

func (c *Client) login() (string, error) {
	data := map[string]interface{}{
		"email":    c.cfg.Username,
		"password": c.cfg.Password,
	}

	body, err := json.Marshal(data)
	if err != nil {
		return "", err
	}

	uri := c.auth.URI + c.endpoints.Login
	req, err := newHttpRequest(http.MethodPost, uri, bytes.NewReader(body), JSONEncoding)
	if err != nil {
		return "", err
	}

	redirect := struct {
		RedirectURL string `json:"redirectUrl"`
	}{}

	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusBadRequest { // TODO: verify good instead of bad
		return "", ErrAuthenticationFailed
	}

	var buf bytes.Buffer
	buf.ReadFrom(resp.Body)
	if err := json.Unmarshal(buf.Bytes(), &redirect); err != nil {
		return "", err
	}

	parsed, err := url.Parse(redirect.RedirectURL)
	return parsed.Query().Get("code"), err
}

func (c *Client) requestAccessToken(accCode string) (string, error) {
	headers := map[string]string{
		"Authorization": "Basic " + c.auth.TokenAuth,
		"Content-type":  "application/x-www-form-urlencoded",
		"User-Agent":    c.auth.UserAgent,
	}

	data := url.Values(map[string][]string{
		"grant_type":   {"authorization_code"},
		"redirect_uri": {c.auth.URI + "/api/v1/user/oauth2/redirect"},
		"code":         {accCode},
	})

	uri := c.auth.URI + c.endpoints.AccessToken
	req, err := newHttpRequest(http.MethodPost, uri, strings.NewReader(data.Encode()), headers)
	if err != nil {
		return "", err
	}

	var tokens struct {
		TokenType   string `json:"token_type"`
		AccessToken string `json:"access_token"`
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var buf bytes.Buffer
	buf.ReadFrom(resp.Body)
	if err := json.Unmarshal(buf.Bytes(), &tokens); err != nil {
		return "", err
	}

	return fmt.Sprintf("%s %s", tokens.TokenType, tokens.AccessToken), err
}

// JSONEncoding specifies application/json
var JSONEncoding = map[string]string{
	"Content-Type": "application/json",
	"Accept":       "application/json",
}
