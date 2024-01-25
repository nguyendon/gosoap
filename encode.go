package gosoap

import (
	"encoding/xml"
	"fmt"
	"github.com/iancoleman/orderedmap"
	"reflect"
	"strconv"
)

var (
	soapPrefix                            = "soap"
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

	hackNamespace := make(map[string]string)
	tokens.startEnvelope()
	if c.Client.HeaderParams != nil {
		tokens.startHeader(c.Client.HeaderName, namespace)
		tokens.recursiveEncode(c.Client.HeaderParams, hackNamespace)
		tokens.endHeader(c.Client.HeaderName)
	}

	err := tokens.startBody(c.Request.Method, namespace)
	if err != nil {
		return err
	}

	if namespace == "http://www.symxchange.generated.symitar.com/filemanagement" {
		hackNamespace["FileName"] = "filemanagementdto:FileName"
	}
	tokens.recursiveEncode(c.Request.Params, hackNamespace)

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

func (tokens *tokenData) recursiveEncode(hm interface{}, hackNamespace map[string]string) {
	v := reflect.ValueOf(hm)

	if instance, ok := hm.(orderedmap.OrderedMap); ok {
		keys := instance.Keys()
		for _, k := range keys {
			child, _ := instance.Get(k)
			kTemp := k
			// Check if k is in the hackNamespace map and if so, use that value instead for local
			if hackNamespace != nil {
				if hackNamespace[k] != "" {
					kTemp = hackNamespace[k]
				}
			}

			t := xml.StartElement{
				Name: xml.Name{
					Space: "",
					Local: kTemp,
				},
			}

			if k == "$attributes" {
				continue
			}

			// k is device information string
			// child is the DeviceInformation Object
			if instance, ok := child.(orderedmap.OrderedMap); ok {
				keys := instance.Keys()
				// check if '$attributes' is in keys
				for _, k := range keys {
					if k == "$attributes" {
						// we have found one with a child attribute
						child, _ := instance.Get(k)
						// For each of its children, fetch the underlying value
						if instance, ok := child.(orderedmap.OrderedMap); ok {
							keys := instance.Keys()
							for _, k := range keys {
								value, _ := instance.Get(k)
								t.Attr = append(t.Attr, xml.Attr{
									Name:  xml.Name{Space: "", Local: k},
									Value: value.(string),
								})
							}
						}
					}
				}
			}

			tokens.data = append(tokens.data, t)
			tokens.recursiveEncode(child, hackNamespace)
			tokens.data = append(tokens.data, xml.EndElement{Name: t.Name})
			continue
		}
		return
	}

	switch v.Kind() {
	case reflect.Map:
		for _, key := range v.MapKeys() {
			t := xml.StartElement{
				Name: xml.Name{
					Space: "",
					Local: key.String(),
				},
			}

			if key.String() == "$attributes" {
				continue
			}

			attributeChild := v.MapIndex(key)

			if attributeChild.IsValid() && attributeChild.Kind() == reflect.Interface {
				// This is deviceinformation
				actualValue := attributeChild.Elem()
				if actualValue.Kind() == reflect.Map {
					attributesKey := reflect.ValueOf("$attributes")
					// we fetch attribute here
					attributesValue := actualValue.MapIndex(attributesKey)
					if attributesValue.IsValid() && attributesValue.Kind() == reflect.Interface {
						underlyingValue := attributesValue.Elem()
						if underlyingValue.Kind() == reflect.Map {
							for iter := underlyingValue.MapRange(); iter.Next(); {
								key := iter.Key().Interface()
								value := iter.Value().Interface()
								t.Attr = append(t.Attr, xml.Attr{
									Name:  xml.Name{Space: "", Local: key.(string)},
									Value: value.(string),
								})
							}
						}
					}
				}
			}

			tokens.data = append(tokens.data, t)
			tokens.recursiveEncode(v.MapIndex(key).Interface(), hackNamespace)
			tokens.data = append(tokens.data, xml.EndElement{Name: t.Name})
		}
	case reflect.Slice:
		for i := 0; i < v.Len(); i++ {
			tokens.recursiveEncode(v.Index(i).Interface(), hackNamespace)
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
			tokens.recursiveEncode(v.Index(1).Interface(), hackNamespace)
			tokens.data = append(tokens.data, xml.EndElement{Name: t.Name})
		}
	case reflect.String:
		content := xml.CharData(v.String())
		tokens.data = append(tokens.data, content)
	case reflect.Struct:
		tokens.data = append(tokens.data, v.Interface())
	case reflect.Int:
	case reflect.Int8:
	case reflect.Int16:
	case reflect.Int32:
	case reflect.Int64:
		content := xml.CharData(strconv.FormatInt(v.Int(), 10))
		tokens.data = append(tokens.data, content)
	case reflect.Uint:
	case reflect.Uint8:
	case reflect.Uint16:
	case reflect.Uint32:
	case reflect.Uint64:
		content := xml.CharData(strconv.FormatUint(v.Uint(), 10))
		tokens.data = append(tokens.data, content)
	case reflect.Float32:
	case reflect.Float64:
		content := xml.CharData(strconv.FormatFloat(v.Float(), 'f', -1, 64))
		tokens.data = append(tokens.data, content)
	case reflect.Bool:
		content := xml.CharData(strconv.FormatBool(v.Bool()))
		tokens.data = append(tokens.data, content)
	default:
		fmt.Println("Can't identify type")
	}
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
				Name:  xml.Name{Space: "", Local: local},
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
			Local: fmt.Sprintf("%s:%s", m, m),
		},
		Attr: []xml.Attr{
			{Name: xml.Name{Space: "", Local: fmt.Sprintf("xmlns:%s", m)}, Value: n},
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
			Local: fmt.Sprintf("%s:%s", m, m),
		},
	}

	tokens.data = append(tokens.data, r, b)
}
