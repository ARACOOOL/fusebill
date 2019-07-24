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
}

type InteractionMode struct {
	Token   string
	ApiHost string
}

type Fusebill struct {
	BaseUrl     string
	Mode        InteractionMode
	Credentials Credentials
	Client      *http.Client
	cookieJar   *cookiejar.Jar
}

var client *http.Client
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

	resp, err := client.Do(request)
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
	if f.cookieJar != nil {
		return nil
	}

	f.cookieJar, _ = cookiejar.New(nil)

	client = &http.Client{
		Jar: f.cookieJar,
	}

	data := url.Values{}
	data.Add("username", f.Credentials.Username)
	data.Add("password", f.Credentials.Password)

	resp, err := client.PostForm(f.BaseUrl+"/api/Login/", data)
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
	request, err := http.NewRequest(r.Method, f.Mode.ApiHost+r.Endpoint, r.Body)
	if err != nil {
		return Response{}, err
	}

	request.Header.Add("Content-Type", "application/json")
	request.Header.Add("Authorization", "Basic "+f.Mode.Token)

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
func NewClient(baseUrl string, mode InteractionMode, credentials Credentials) *Fusebill {
	return &Fusebill{
		BaseUrl:     baseUrl,
		Mode:        mode,
		Credentials: credentials,
		Client:      &http.Client{},
	}
}
