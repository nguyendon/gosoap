package gosoap

import (
  "bytes"
	"encoding/xml"
	"fmt"
	"reflect"
)

var (
	soapPrefix = "soap"
	customEnvelopeAttrs map[string]string = nil
)

// SetCustomEnvelope define customizated envelope
func SetCustomEnvelope(prefix string, attrs map[string]string) {
	soapPrefix = prefix
	if attrs != nil {
		customEnvelopeAttrs = attrs
	}
}

// MarshalXML envelope the body and encode to xml
func (c process) MarshalXML(e *xml.Encoder, _ xml.StartElement) error {
  fmt.Println("process.MarshalXML")
	tokens := &tokenData{}

	//start envelope
	if c.Client.Definitions == nil {
		return fmt.Errorf("definitions is nil")
	}

	namespace := ""
	if c.Client.Definitions.Types != nil {
		schema := c.Client.Definitions.Types[0].XsdSchema[0]
		namespace = schema.TargetNamespace
		if namespace == "" && len(schema.Imports) > 0 {
			namespace = schema.Imports[0].Namespace
		}
	}

	tokens.startEnvelope()
	if c.Client.HeaderParams != nil {
		tokens.startHeader(c.Client.HeaderName, namespace)
		tokens.recursiveEncode(c.Client.HeaderParams)
		tokens.endHeader(c.Client.HeaderName)
	}

	err := tokens.startBody(c.Request.Method, namespace)
	if err != nil {
		return err
	}

	tokens.recursiveEncode(c.Request.Params)

	//end envelope
	tokens.endBody(c.Request.Method)
	tokens.endEnvelope()

	for _, t := range tokens.data {
		err := e.EncodeToken(t)
		if err != nil {
			return err
		}
	}

	return e.Flush()
}

type tokenData struct {
	data []xml.Token
}

// Returns true if this iteration is on an attribute
func (tokens *tokenData) recursiveEncode(hm interface{}) bool {
	v := reflect.ValueOf(hm)
  isAttribute := false

	switch v.Kind() {
	case reflect.Map:
		for _, key := range v.MapKeys() {
			t := xml.StartElement{
				Name: xml.Name{
					Space: "",
					Local: key.String(),
				},
			}

    // Check for $attributes and add them to the start element
    /*
    if attrs, ok := data["$attributes"].(map[string]interface{}); ok {
      for attrName, attrValue := range attrs {
        attr := xml.Attr{Name: xml.Name{Local: attrName}, Value: fmt.Sprintf("%v", attrValue)}
        start.Attr = append(start.Attr, attr)
      }
    }
    */


      // Check if v.MapIndex(key) is a map[string]interface{}
      // If it is, then check if it has a $attributes key
      // If it does, then we need to treat this as an attribute
      fmt.Println(key.String())

      // check that the next key is a map[string]interface{}
      hasAttribute := false
      fmt.Println(key.String())
      fmt.Println(v.MapIndex(key).Kind())
      fmt.Println(v.MapIndex(key).Kind() == reflect.Map)
      if (key.String() == "DeviceInformation") {
        if (v.MapIndex(key).Interface().(map[string]interface{})["$attributes"] != nil) {
          fmt.Println("has attribute")
          hasAttribute = true
          fmt.Println("short circuiting")
          // It has an attribute to add it to the current key element
          // Add the attributes to the current key element
          for attrName, attrValue := range v.MapIndex(key).Interface().(map[string]interface{})["$attributes"].(map[string]interface{}) {
            fmt.Println(attrName)
            fmt.Println(attrValue)
            attr := xml.Attr{Name: xml.Name{Local: attrName}, Value: fmt.Sprintf("%v", attrValue)}
            t.Attr = append(t.Attr, attr)
            fmt.Println("appended")
            fmt.Println(t.Attr)
          }
        }
      }

			tokens.data = append(tokens.data, t)
      if (!hasAttribute) {
        tokens.recursiveEncode(v.MapIndex(key).Interface())
      }
			tokens.data = append(tokens.data, xml.EndElement{Name: t.Name})
		}
	case reflect.Slice:
		for i := 0; i < v.Len(); i++ {
			tokens.recursiveEncode(v.Index(i).Interface())
		}
	case reflect.Array:
		if v.Len() == 2 {
			label := v.Index(0).Interface()
			t := xml.StartElement{
				Name: xml.Name{
					Space: "",
					Local: label.(string),
				},
			}

			tokens.data = append(tokens.data, t)
			tokens.recursiveEncode(v.Index(1).Interface())
			tokens.data = append(tokens.data, xml.EndElement{Name: t.Name})
		}
	case reflect.String:
		content := xml.CharData(v.String())
		tokens.data = append(tokens.data, content)
	case reflect.Struct:
		tokens.data = append(tokens.data, v.Interface())
	}

  return isAttribute
}

func (tokens *tokenData) startEnvelope() {
	e := xml.StartElement{
		Name: xml.Name{
			Space: "",
			Local: fmt.Sprintf("%s:Envelope", soapPrefix),
		},
	}

	if customEnvelopeAttrs == nil {
		e.Attr = []xml.Attr{
			{Name: xml.Name{Space: "", Local: "xmlns:xsi"}, Value: "http://www.w3.org/2001/XMLSchema-instance"},
			{Name: xml.Name{Space: "", Local: "xmlns:xsd"}, Value: "http://www.w3.org/2001/XMLSchema"},
			{Name: xml.Name{Space: "", Local: "xmlns:soap"}, Value: "http://schemas.xmlsoap.org/soap/envelope/"},
		}
	} else {
		e.Attr = make([]xml.Attr, 0)
		for local, value := range customEnvelopeAttrs {
			e.Attr = append(e.Attr, xml.Attr{
				Name: xml.Name{Space: "", Local: local},
				Value: value,
			})
		}
	}

	tokens.data = append(tokens.data, e)
}

func (tokens *tokenData) endEnvelope() {
	e := xml.EndElement{
		Name: xml.Name{
			Space: "",
			Local: fmt.Sprintf("%s:Envelope", soapPrefix),
		},
	}

	tokens.data = append(tokens.data, e)
}

func (tokens *tokenData) startHeader(m, n string) {
	h := xml.StartElement{
		Name: xml.Name{
			Space: "",
			Local: fmt.Sprintf("%s:Header", soapPrefix),
		},
	}

	if m == "" || n == "" {
		tokens.data = append(tokens.data, h)
		return
	}

	r := xml.StartElement{
		Name: xml.Name{
			Space: "",
			Local: m,
		},
		Attr: []xml.Attr{
			{Name: xml.Name{Space: "", Local: "xmlns"}, Value: n},
		},
	}

	tokens.data = append(tokens.data, h, r)

	return
}

func (tokens *tokenData) endHeader(m string) {
	h := xml.EndElement{
		Name: xml.Name{
			Space: "",
			Local: fmt.Sprintf("%s:Header", soapPrefix),
		},
	}

	if m == "" {
		tokens.data = append(tokens.data, h)
		return
	}

	r := xml.EndElement{
		Name: xml.Name{
			Space: "",
			Local: m,
		},
	}

	tokens.data = append(tokens.data, r, h)
}

func (tokens *tokenData) startBody(m, n string) error {
	b := xml.StartElement{
		Name: xml.Name{
			Space: "",
			Local: fmt.Sprintf("%s:Body", soapPrefix),
		},
	}

	if m == "" || n == "" {
		return fmt.Errorf("method or namespace is empty")
	}

	r := xml.StartElement{
		Name: xml.Name{
			Space: "",
			Local: m,
		},
		Attr: []xml.Attr{
			{Name: xml.Name{Space: "", Local: "xmlns"}, Value: n},
		},
	}

	tokens.data = append(tokens.data, b, r)

	return nil
}

// endToken close body of the envelope
func (tokens *tokenData) endBody(m string) {
	b := xml.EndElement{
		Name: xml.Name{
			Space: "",
			Local: fmt.Sprintf("%s:Body", soapPrefix),
		},
	}

	r := xml.EndElement{
		Name: xml.Name{
			Space: "",
			Local: m,
		},
	}

	tokens.data = append(tokens.data, r, b)
}

// customMarshalXML handles the map and encodes it into XML.
// If a "$attributes" key is found, its value is treated as attributes.

//func (c process) MarshalXML(e *xml.Encoder, _ xml.StartElement) error {
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
