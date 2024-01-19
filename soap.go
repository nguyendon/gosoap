package gosoap

import (
  "bytes"
  "encoding/xml"
  "errors"
  "fmt"
  "io"
  "io/ioutil"
  "log"
  "net/http"
  "net/http/httputil"
  "net/url"
  "strings"
  "sync"
  "time"

  "golang.org/x/net/html/charset"
)

type SoapParams interface{}

// HeaderParams holds params specific to the header
type HeaderParams map[string]interface{}

// Params type is used to set the params in soap request
type Params map[string]interface{}
type ArrayParams [][2]interface{}
type SliceParams []interface{}

type DumpLogger interface {
  LogRequest(method string, dump []byte)
  LogResponse(method string, dump []byte)
}

type fmtLogger struct{}

func (l *fmtLogger) LogRequest(method string, dump []byte) {
  fmt.Printf("Request:\n%v\n----\n", string(dump))
}

func (l *fmtLogger) LogResponse(method string, dump []byte) {
  fmt.Printf("Response:\n%v\n----\n", string(dump))
}

// Config config the Client
type Config struct {
  Dump   bool
  Logger DumpLogger
}

// SoapClient return new *Client to handle the requests with the WSDL
func SoapClient(wsdl string, httpClient *http.Client) (*Client, error) {
  return SoapClientWithConfig(wsdl, httpClient, &Config{Dump: false, Logger: &fmtLogger{}})
}

// SoapClientWithConfig return new *Client to handle the requests with the WSDL
func SoapClientWithConfig(wsdl string, httpClient *http.Client, config *Config) (*Client, error) {
  _, err := url.Parse(wsdl)
  if err != nil {
    return nil, err
  }

  if httpClient == nil {
    httpClient = &http.Client{}
  }

  if config.Logger == nil {
    config.Logger = &fmtLogger{}
  }

  c := &Client{
    wsdl:       wsdl,
    config:     config,
    HTTPClient: httpClient,
    AutoAction: false,
  }

  return c, nil
}

// Client struct hold all the information about WSDL,
// request and response of the server
type Client struct {
  HTTPClient   *http.Client
  AutoAction   bool
  URL          string
  HeaderName   string
  HeaderParams SoapParams
  Definitions  *wsdlDefinitions
  // Must be set before first request otherwise has no effect, minimum is 15 minutes.
  RefreshDefinitionsAfter time.Duration
  Username                string
  Password                string

  once                 sync.Once
  definitionsErr       error
  onRequest            sync.WaitGroup
  onDefinitionsRefresh sync.WaitGroup
  wsdl                 string
  config               *Config
}

// Call call's the method m with Params p
func (c *Client) Call(m string, p SoapParams) (res *Response, err error) {
  return c.Do(NewRequest(m, p))
}

// CallByStruct call's by struct
func (c *Client) CallByStruct(s RequestStruct) (res *Response, err error) {
  req, err := NewRequestByStruct(s)
  if err != nil {
    return nil, err
  }

  return c.Do(req)
}

func (c *Client) waitAndRefreshDefinitions(d time.Duration) {
  for {
    time.Sleep(d)
    c.onRequest.Wait()
    c.onDefinitionsRefresh.Add(1)
    c.initWsdl()
    c.onDefinitionsRefresh.Done()
  }
}

func (c *Client) initWsdl() {
  c.Definitions, c.definitionsErr = getWsdlDefinitions(c.wsdl, c.HTTPClient)
  if c.definitionsErr == nil {
    c.URL = strings.TrimSuffix(c.Definitions.TargetNamespace, "/")
  }
}

// SetWSDL set WSDL url
func (c *Client) SetWSDL(wsdl string) {
  c.onRequest.Wait()
  c.onDefinitionsRefresh.Wait()
  c.onRequest.Add(1)
  c.onDefinitionsRefresh.Add(1)
  defer c.onRequest.Done()
  defer c.onDefinitionsRefresh.Done()
  c.wsdl = wsdl
  c.initWsdl()
}

// Do Process Soap Request
func (c *Client) Do(req *Request) (res *Response, err error) {
  c.onDefinitionsRefresh.Wait()
  c.onRequest.Add(1)
  defer c.onRequest.Done()

  c.once.Do(func() {
    c.initWsdl()
    // 15 minute to prevent abuse.
    if c.RefreshDefinitionsAfter >= 15*time.Minute {
      go c.waitAndRefreshDefinitions(c.RefreshDefinitionsAfter)
    }
  })

  if c.definitionsErr != nil {
    return nil, c.definitionsErr
  }

  if c.Definitions == nil {
    return nil, errors.New("wsdl definitions not found")
  }

  if c.Definitions.Services == nil {
    return nil, errors.New("No Services found in wsdl definitions")
  }

  p := &process{
    Client:     c,
    Request:    req,
    SoapAction: c.Definitions.GetSoapActionFromWsdlOperation(req.Method),
  }

  if p.SoapAction == "" && c.AutoAction {
    p.SoapAction = fmt.Sprintf("%s/%s/%s", c.URL, c.Definitions.Services[0].Name, req.Method)
  }

  fmt.Println("req")
  fmt.Println(req)
  fmt.Println("req.Method")
  fmt.Println(req.Method)
  fmt.Println("req.Params")
  fmt.Println(req.Params)
  fmt.Println("p1")
  fmt.Println(p)
  p.Payload, err = xml.MarshalIndent(p, "", "    ")
  if err != nil {
    return nil, err
  }

  b, err := p.doRequest(c.Definitions.Services[0].Ports[0].SoapAddresses[0].Location)
  if err != nil {
    return nil, ErrorWithPayload{err, p.Payload}
  }

  var soap SoapEnvelope
  // err = xml.Unmarshal(b, &soap)
  // error: xml: encoding "ISO-8859-1" declared but Decoder.CharsetReader is nil
  // https://stackoverflow.com/questions/6002619/unmarshal-an-iso-8859-1-xml-input-in-go
  // https://github.com/golang/go/issues/8937

  decoder := xml.NewDecoder(bytes.NewReader(b))
  decoder.CharsetReader = charset.NewReaderLabel
  err = decoder.Decode(&soap)

  res = &Response{
    Body:    soap.Body.Contents,
    Header:  soap.Header.Contents,
    Payload: p.Payload,
  }
  if err != nil {
    return res, ErrorWithPayload{err, p.Payload}
  }

  return res, nil
}

type process struct {
  Client     *Client
  Request    *Request
  SoapAction string
  Payload    []byte
}

// doRequest makes new request to the server using the c.Method, c.URL and the body.
// body is enveloped in Do method
func (p *process) doRequest(url string) ([]byte, error) {
  // At this point p.Payload is the byte representation of the envelope
  req, err := http.NewRequest("POST", url, bytes.NewBuffer(p.Payload))
  // At this point it is in the envelope as a request body
  if err != nil {
    return nil, err
  }

  if p.Client.config != nil && p.Client.config.Dump {
    dump, err := httputil.DumpRequestOut(req, true)
    if err != nil {
      return nil, err
    }
    p.Client.config.Logger.LogRequest(p.Request.Method, dump)
  }

  if p.Client.Username != "" && p.Client.Password != "" {
    req.SetBasicAuth(p.Client.Username, p.Client.Password)
  }

  req.ContentLength = int64(len(p.Payload))

  req.Header.Add("Content-Type", "text/xml;charset=UTF-8")
  req.Header.Add("Accept", "text/xml")
  if p.SoapAction != "" {
    req.Header.Add("SOAPAction", p.SoapAction)
  }

  resp, err := p.httpClient().Do(req)
  if err != nil {
    return nil, err
  }
  defer resp.Body.Close()

  if p.Client.config != nil && p.Client.config.Dump {
    dump, err := httputil.DumpResponse(resp, true)
    if err != nil {
      return nil, err
    }
    p.Client.config.ogger.LogResponse(p.Request.Method, dump)
  }

  // Unmarshal response body
  responseBody, err := ioutil.ReadAll(resp.Body)
  if err != nil {
    log.Fatal(err)
  }

  if resp.StatusCode < 200 || resp.StatusCode >= 400 {
    if !(p.Client.config != nil && p.Client.config.Dump) {
      _, err := io.Copy(ioutil.Discard, resp.Body)
      if err != nil {
        return nil, err
      }
    }
    return nil, errors.New("unexpected status code: " + resp.Status)
  }

  return ioutil.ReadAll(resp.Body)
}

func (p *process) httpClient() *http.Client {
  if p.Client.HTTPClient != nil {
    return p.Client.HTTPClient
  }
  return http.DefaultClient
}

// ErrorWithPayload error payload schema
type ErrorWithPayload struct {
  error
  Payload []byte
}

// GetPayloadFromError returns the payload of a ErrorWithPayload
func GetPayloadFromError(err error) []byte {
  if err, ok := err.(ErrorWithPayload); ok {
    return err.Payload
  }
  return nil
}

// SoapEnvelope struct
type SoapEnvelope struct {
  XMLName struct{} `xml:"Envelope"`
  Header  SoapHeader
  Body    SoapBody
}

// SoapHeader struct
type SoapHeader struct {
  XMLName  struct{} `xml:"Header"`
  Contents []byte   `xml:",innerxml"`
}

// SoapBody struct
type SoapBody struct {
  XMLName  struct{} `xml:"Body"`
  Contents []byte   `xml:",innerxml"`
}

// customMarshalXML handles the map and encodes it into XML.
// If a "$attributes" key is found, its value is treated as attributes.
func customMarshalXML(data map[string]interface{}, parentName string) ([]byte, error) {
  // Create an encoder and a buffer to hold the XML
  buf := new(bytes.Buffer)
  encoder := xml.NewEncoder(buf)

  start := xml.StartElement{Name: xml.Name{Local: parentName}}

  // Check for $attributes and add them to the start element
  if attrs, ok := data["$attributes"].(map[string]interface{}); ok {
    for attrName, attrValue := range attrs {
      attr := xml.Attr{Name: xml.Name{Local: attrName}, Value: fmt.Sprintf("%v", attrValue)}
      start.Attr = append(start.Attr, attr)
    }
  }

  // Start the element
  encoder.EncodeToken(start)

  // Encode child elements
  for key, value := range data {
    if key == "$attributes" {
      continue
    }

    switch child := value.(type) {
    case map[string]interface{}:
      childXML, err := customMarshalXML(child, key)
      if err != nil {
        return nil, err
      }
      buf.Write(childXML)
    default:
      // Handle other types as needed (e.g., string, int, slices)
      encoder.EncodeElement(child, xml.StartElement{Name: xml.Name{Local: key}})
    }
  }

  // End the element
  encoder.EncodeToken(xml.EndElement{Name: start.Name})

  // Flush to ensure all XML is written to the buffer
  encoder.Flush()

  return buf.Bytes(), nil
}
