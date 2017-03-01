package main

import (
	"bufio"
	"crypto/md5"
	"flag"
	"fmt"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"log"
	"net/http"
	net_url "net/url"
	"os"
	"regexp"
	"strings"
	"time"
)

func check(e error) {
	if e != nil {
		log.Fatalf("error: %v", e)
		panic(e)
	}
}

type Version struct {
	host          string
	url           string
	version_hash  string
	header_hash   string
	includes_hash string
	includes_list string
	updated       int64
	success       bool
	error_message string
}

type Config struct {
	Urls []string
	Parse_headers bool
}

type Options struct {
	verbose     bool
	debug       bool
	config_file string
	urls        []string
}

var options Options
var config Config

func ParseArgs() {
	options = Options{}

	// Define Flags
	configFile := "./config/config.yaml"
	cPtr := flag.String("c", configFile, "[optional] set a custom config file")
	dPtr := flag.Bool("d", false, "run in debug mode")
	debugPtr := flag.Bool("debug", false, "")
	vPtr := flag.Bool("v", false, "whether to run verbosely")
	verbosePtr := flag.Bool("verbose", false, "")
	uPtr := flag.String("u", "", "A single URL to Process")

	// Parse Flags
	flag.Parse()
	options.verbose = *verbosePtr || *vPtr
	options.debug = *debugPtr || *dPtr
	options.config_file = *cPtr
	if len(*uPtr) > 0 {
		options.urls = []string{*uPtr}
	} else {
		options.urls = []string{}
	}
}

func ParseConfig() Config {
	// read file into memory
	configData, err := ioutil.ReadFile(options.config_file)
	check(err)

	// parse yaml
	parsedConfig := Config{}
	err = yaml.Unmarshal([]byte(configData), &parsedConfig)
	check(err)
	return parsedConfig
}

func main() {
	ParseArgs()
	config = ParseConfig()

	var urls []string
	if (len(options.urls) > 0) {
		urls = options.urls
	} else {
		urls = config.Urls
	}

	// Store all results
	versions := make([]*Version, len(urls))

	// Concurrently pull data
	concurrency := 20
	if options.debug {
		concurrency = 1
	}

	sem := make(chan bool, concurrency)
	for i, url := range urls {
		sem <- true
		go func(i int, url string) {
			defer func() { <-sem }()
			// Process This Url, 5 at a time
			versions[i] = ProcessUrl(url)

		}(i, url)

	}
	// wait for goroutines to finish
	for i := 0; i < cap(sem); i++ {
		sem <- true
	}

	// Persist any updates
	UpdateDynamo(versions)

	for _, v := range versions {
		if !v.success || options.verbose {
			fmt.Println("host request status", v.host)
			fmt.Printf("- success:       %v\n", v.success)
			fmt.Printf("- url:           %v\n", v.url)
			fmt.Printf("- error message: %v\n", v.error_message)
		}
	}
	fmt.Println("Update Complete")
}

func ProcessUrl(url string) *Version {
	if options.verbose {
		fmt.Println("Processing Url:", url)
	}

	version := Version{}
	version.url = url
	version.host = GetHost(url)
	version.success = false

	// Get url data
	resp, err := http.Get(url)

	// Errors don't indicate a version change.  Fail on them.
	if err != nil || resp.StatusCode != 200 {
		statuscode := 0
		if resp != nil {
			statuscode = resp.StatusCode
		}
		version.error_message = fmt.Sprintf("msg: %v, status code: %v", err, statuscode)
		return &version
	}

	data, err := ioutil.ReadAll(resp.Body)
	body := string(data)
	resp.Body.Close()

	// Hash Content That Suggests Versioning
	includes_hash, includes_list := GetIncludesHash(body)
	header_hash := GetHeaderHash(body)
	version_hash := fmt.Sprintf("%x", md5.Sum([]byte(includes_hash+header_hash)))

	// Verbose & Debug Info
	if options.verbose {
		fmt.Println("- includes hash: ", includes_hash)
		fmt.Println("- header hash: ", header_hash)
		fmt.Println("- version hash: ", version_hash)
	}

	if options.debug {
		reader := bufio.NewReader(os.Stdin)
		fmt.Println("<Press Enter To Continue>")
		reader.ReadString('\n')
	}

	version.version_hash = version_hash
	version.header_hash = header_hash
	version.includes_hash = includes_hash
	version.includes_list = includes_list
	version.success = true
	version.updated = time.Now().Unix()
	return &version
}

func GetIncludesHash(body string) (string, string) {

	// gather up all js/css includes
	re := regexp.MustCompile("(src|href)=\"([^\"]+\\.(js|css)[^\"]*)\"")
	matches := re.FindAllStringSubmatch(string(body), -1)

	// remove intermediate matches
	includes := []string{}
	for _, m := range matches {
		includes = append(includes, m[2])
	}

	if len(includes) == 0 {
		return "no_includes", "no_includes"
	}

	// Join strings into one object that we can hash
	includes_list := strings.Join(includes, ",")

	if options.verbose {
		intermediate := strings.Join(includes, ",\n - ")
		fmt.Println("- includes:")
		fmt.Println(" -", intermediate)
	}

	// return our hash
	hash := md5.Sum([]byte(includes_list))
	return fmt.Sprintf("%x", hash), includes_list
}

func UpdateDynamo(versions []*Version) {

	if options.debug {
		fmt.Println("- skipping dynamo update in debug mode")
		return
	}

	ddb := GetDDB()

	for _, v := range versions {
		// skip any failed version updates
		if !v.success {
			continue
		}

		// Get last version from dynamo
		last_version, update_count := GetLastVersion(ddb, v.host)
		if strings.Compare(last_version, v.version_hash) != 0 {
			fmt.Println("- update found for", v.host)
			fmt.Println("  - last version: ", last_version)
			fmt.Println("  - update count: ", update_count)
			// Save
			update_count++
			UpdateVersion(ddb, v, update_count)
		}
	}
}

func GetHeaderHash(body string) string {
	// collect <head></head>
	re := regexp.MustCompile("(?si)<head>(.*)</head>")
	matches := re.FindAllStringSubmatch(body, -1)
	// end if no header is defined
	if len(matches) == 0 || !config.Parse_headers {
		return "no_header"
	}

	// cleanup
	head := matches[0][1]
	// remove all whitespace & newlines and make lowercase for better hashing
	head = strings.ToLower(strings.Join(strings.Fields(head), ""))

	// only select meta and link tags.  other tags might be noisy.
	re = regexp.MustCompile("<(meta|link)([^>]*)>")
	matches = re.FindAllStringSubmatch(head, -1)

	head_tags := make([]string, len(matches))
	for i, m := range matches {
		if strings.Contains(m[2], "csrf") {
			// Ignore CSRF tokens
			head_tags[i] = "removed"
		} else if strings.Contains(m[2], "_pad") {
			// Ignore custom padding
			head_tags[i] = "removed"
		} else {
			// otherwise use the filed for versioning
			head_tags[i] = m[2]
		}
	}
	hashable := strings.Join(head_tags, "\n")

	// now hash
	hash := md5.Sum([]byte(hashable))
	return fmt.Sprintf("%x", hash)
}

func GetHost(url string) string {
	host, err := net_url.Parse(url)
	check(err)
	return host.Host
}
