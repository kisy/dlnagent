package dlna

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"text/template"
)

const soapEnvelope = `<?xml version="1.0" encoding="utf-8"?>
<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/" s:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/">
  <s:Body>
    {{.Body}}
  </s:Body>
</s:Envelope>`

const setAVTransportURIBody = `<u:SetAVTransportURI xmlns:u="urn:schemas-upnp-org:service:AVTransport:1">
  <InstanceID>0</InstanceID>
  <CurrentURI>{{.MediaURL}}</CurrentURI>
  <CurrentURIMetaData>{{.MetaData}}</CurrentURIMetaData>
</u:SetAVTransportURI>`

const playBody = `<u:Play xmlns:u="urn:schemas-upnp-org:service:AVTransport:1">
  <InstanceID>0</InstanceID>
  <Speed>1</Speed>
</u:Play>`

func Play(controlURL, mediaURL, title string) error {
	// 1. SetAVTransportURI
	metaData := ""
	if title != "" {
		// Simple DIDL-Lite metadata
		// We construct the raw XML first, then escape it.
		metaData = fmt.Sprintf(`<DIDL-Lite xmlns="urn:schemas-upnp-org:metadata-1-0/DIDL-Lite/" xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:upnp="urn:schemas-upnp-org:metadata-1-0/upnp/"><item id="0" parentID="0" restricted="1"><dc:title>%s</dc:title><upnp:class>object.item.videoItem</upnp:class><res protocolInfo="http-get:*:*:*">%s</res></item></DIDL-Lite>`, title, mediaURL)

		// Escape the XML string to be embedded in the SOAP XML
		var buf bytes.Buffer
		if err := xml.EscapeText(&buf, []byte(metaData)); err == nil {
			metaData = buf.String()
		}
	}

	if err := sendSOAPAction(controlURL, "SetAVTransportURI", setAVTransportURIBody, map[string]string{"MediaURL": mediaURL, "MetaData": metaData}); err != nil {
		return fmt.Errorf("SetAVTransportURI failed: %w", err)
	}

	// 2. Play
	if err := sendSOAPAction(controlURL, "Play", playBody, nil); err != nil {
		return fmt.Errorf("Play failed: %w", err)
	}

	return nil
}

func sendSOAPAction(controlURL, action, bodyTmpl string, data interface{}) error {
	// Render body
	t := template.Must(template.New("body").Parse(bodyTmpl))
	var bodyBytes bytes.Buffer
	if err := t.Execute(&bodyBytes, data); err != nil {
		return err
	}

	// Render envelope
	tEnv := template.Must(template.New("envelope").Parse(soapEnvelope))
	var envelopeBytes bytes.Buffer
	if err := tEnv.Execute(&envelopeBytes, map[string]string{"Body": bodyBytes.String()}); err != nil {
		return err
	}

	req, err := http.NewRequest("POST", controlURL, &envelopeBytes)
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "text/xml; charset=\"utf-8\"")
	req.Header.Set("SOAPAction", fmt.Sprintf("\"urn:schemas-upnp-org:service:AVTransport:1#%s\"", action))

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("SOAP request failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}
