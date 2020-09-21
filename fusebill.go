package fusebill

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strconv"
	"sync"
	"time"
)

type Invoice struct {
	OutstandingBalance float64 `json:"outstandingBalance"`
}

type WriteOff struct {
	InvoiceId int     `json:"invoiceId"`
	Amount    float64 `json:"amount"`
}

type RequestDetails struct {
	Method   string
	Endpoint string
	Body     io.Reader
}

type Response struct {
	Body       []byte
	StatusCode int
}

type Credentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Token    string
}

type Fusebill struct {
	BaseUrl     string
	Credentials Credentials
	Client      *http.Client
	cookieJar   *cookiejar.Jar
}

var mux sync.Mutex

// WriteOff writes a invoice off
func (f *Fusebill) WriteOff(invoiceID string, balance float64) error {
	if balance <= 0 {
		return errors.New(fmt.Sprintf("Invoice %s: outstandingBalance is %.2f", invoiceID, balance))
	}

	mux.Lock()
	err := f.login()
	if err != nil {
		mux.Unlock()
		return err
	}
	mux.Unlock()

	i, _ := strconv.Atoi(invoiceID)
	data := &WriteOff{InvoiceId: i, Amount: balance}
	r, _ := json.Marshal(data)

	request, err := http.NewRequest("POST", f.BaseUrl+"/api/invoices/writeoff", bytes.NewBuffer(r))
	if err != nil {
		return err
	}

	request.Header.Set("Content-Type", "application/json")

	resp, err := f.Client.Do(request)
	if err != nil {
		return err
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := ioutil.ReadAll(resp.Body)
		return fmt.Errorf("server returns %d; message %s", resp.StatusCode, body)
	}

	return nil
}

// Login user
func (f *Fusebill) login() error {
	if f.cookieJar == nil {
		return errors.New("cookie jar should be set")
	}

	data := url.Values{}
	data.Add("username", f.Credentials.Username)
	data.Add("password", f.Credentials.Password)

	resp, err := f.Client.PostForm(f.BaseUrl+"/api/Login/", data)
	if err != nil {
		return err
	}

	defer resp.Body.Close()
	_, _ = ioutil.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return errors.New("unable to login, check you login and password")
	}

	return nil
}

func (f *Fusebill) GetInvoiceBalance(invoiceID string) (float64, error) {
	resp, err := f.SendRequest(RequestDetails{"GET", "/invoices/" + invoiceID, nil})
	if err != nil {
		return 0, err
	}

	invoice := &Invoice{}
	_ = json.Unmarshal(resp.Body, invoice)
	return invoice.OutstandingBalance, nil
}

//SendRequest sends request to the specific endpoint
func (f *Fusebill) SendRequest(r RequestDetails) (Response, error) {
	request, err := http.NewRequest(r.Method, f.BaseUrl+r.Endpoint, r.Body)
	if err != nil {
		return Response{}, err
	}

	request.Header.Add("Content-Type", "application/json")
	request.Header.Add("Authorization", "Basic "+f.Credentials.Token)

	resp, err := f.Client.Do(request)
	if err != nil {
		return Response{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		content, _ := ioutil.ReadAll(resp.Body)
		return Response{}, errors.New(fmt.Sprintf("Request failed with the status code: %d, content: %s", resp.StatusCode, string(content)))
	}

	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return Response{}, err
	}

	return Response{Body: b, StatusCode: resp.StatusCode}, nil
}

// NewClient returns the new fusebill client
func NewClient(mode string, credentials Credentials) *Fusebill {
	var baseUrl string
	if mode == "production" {
		baseUrl = "https://secure.fusebill.com/v1"
	} else {
		baseUrl = "https://stg-secure.fusebill.com/v1"
	}

	return &Fusebill{
		BaseUrl:     baseUrl,
		Credentials: credentials,
		Client: &http.Client{
			Timeout: time.Second * 5,
		},
	}
}

// NewPrivateClient returns the new fusebill client for the Private API
func NewPrivateClient(mode string, credentials Credentials) *Fusebill {
	var baseUrl string
	if mode == "production" {
		baseUrl = "https://secure.fusebill.com"
	} else {
		baseUrl = "https://stg-secure.fusebill.com"
	}

	client := &Fusebill{
		BaseUrl:     baseUrl,
		Credentials: credentials,
		Client: &http.Client{
			Timeout: time.Second * 5,
		},
	}

	client.cookieJar, _ = cookiejar.New(nil)
	client.Client.Jar = client.cookieJar

	return client
}
