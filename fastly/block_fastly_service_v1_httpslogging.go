package fastly

import (
	"fmt"
	"log"

	gofastly "github.com/fastly/go-fastly/fastly"
	"github.com/hashicorp/terraform-plugin-sdk/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/helper/validation"
)

var httpsloggingSchema = &schema.Schema{
	Type:     schema.TypeSet,
	Optional: true,
	Elem: &schema.Resource{
		Schema: map[string]*schema.Schema{
			// Required fields
			"name": {
				Type:        schema.TypeString,
				Required:    true,
				Description: "The unique name of the HTTPS logging endpoint",
			},
			"url": {
				Type:         schema.TypeString,
				Required:     true,
				Description:  "URL that log data will be sent to. Must use the https protocol.",
				ValidateFunc: validateHTTPSURL(),
			},

			// Optional fields
			"request_max_entries": {
				Type:        schema.TypeInt,
				Optional:    true,
				Description: "The maximum number of logs sent in one request.",
			},

			"request_max_bytes": {
				Type:        schema.TypeInt,
				Optional:    true,
				Description: "The maximum number of bytes sent in one request.",
			},

			"content_type": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "Content-Type header sent with the request.",
			},

			"header_name": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "Custom header sent with the request.",
			},

			"header_value": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "Value of the custom header sent with the request.",
			},

			"method": {
				Type:         schema.TypeString,
				Optional:     true,
				Default:      "POST",
				Description:  "HTTP method used for request.",
				ValidateFunc: validation.StringInSlice([]string{"POST", "PUT"}, false),
			},

			// NOTE: The `json_format` field's documented type is string, but it should likely be an integer.
			"json_format": {
				Type:         schema.TypeString,
				Optional:     true,
				Default:      "0",
				Description:  "Formats log entries as JSON. Can be either disabled (`0`), array of json (`1`), or newline delimited json (`2`).",
				ValidateFunc: validation.StringInSlice([]string{"0", "1", "2"}, false),
			},

			"tls_ca_cert": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "A secure certificate to authenticate the server with. Must be in PEM format.",
				Sensitive:   true,
			},

			"tls_client_cert": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "The client certificate used to make authenticated requests. Must be in PEM format.",
				Sensitive:   true,
			},

			"tls_client_key": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "The client private key used to make authenticated requests. Must be in PEM format.",
				Sensitive:   true,
			},

			"tls_hostname": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "The hostname used to verify the server's certificate. It can either be the Common Name (CN) or a Subject Alternative Name (SAN).",
			},

			"format": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "Apache-style string or VCL variables to use for log formatting.",
			},

			"format_version": {
				Type:         schema.TypeInt,
				Optional:     true,
				Default:      2,
				Description:  "The version of the custom logging format used for the configured endpoint. Can be either 1 or 2. (default: 2)",
				ValidateFunc: validateLoggingFormatVersion(),
			},

			"message_type": {
				Type:         schema.TypeString,
				Optional:     true,
				Default:      "blank",
				Description:  "How the message should be formatted",
				ValidateFunc: validateLoggingMessageType(),
			},

			"placement": {
				Type:         schema.TypeString,
				Optional:     true,
				Description:  "Where in the generated VCL the logging call should be placed",
				ValidateFunc: validateLoggingPlacement(),
			},

			"response_condition": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "The name of the condition to apply",
			},
		},
	},
}

func processHTTPS(d *schema.ResourceData, conn *gofastly.Client, latestVersion int) error {
	serviceID := d.Id()
	oh, nh := d.GetChange("httpslogging")

	if oh == nil {
		oh = new(schema.Set)
	}
	if nh == nil {
		nh = new(schema.Set)
	}

	ohs := oh.(*schema.Set)
	nhs := nh.(*schema.Set)

	removeHTTPSLogging := ohs.Difference(nhs).List()
	addHTTPSLogging := nhs.Difference(ohs).List()

	// DELETE old HTTPS logging endpoints
	for _, oRaw := range removeHTTPSLogging {
		of := oRaw.(map[string]interface{})
		opts := buildDeleteHTTPS(of, serviceID, latestVersion)

		log.Printf("[DEBUG] Fastly HTTPS logging endpoint removal opts: %#v", opts)

		if err := deleteHTTPS(conn, opts); err != nil {
			return err
		}
	}

	// POST new/updated HTTPS logging endponts
	for _, nRaw := range addHTTPSLogging {
		hf := nRaw.(map[string]interface{})
		opts := buildCreateHTTPS(hf, serviceID, latestVersion)

		log.Printf("[DEBUG] Fastly HTTPS logging addition opts: %#v", opts)

		if err := createHTTPS(conn, opts); err != nil {
			return err
		}
	}

	return nil
}

func readHTTPS(conn *gofastly.Client, d *schema.ResourceData, s *gofastly.ServiceDetail) error {
	// refresh HTTPS
	log.Printf("[DEBUG] Refreshing HTTPS logging endpoints for (%s)", d.Id())
	httpsList, err := conn.ListHTTPS(&gofastly.ListHTTPSInput{
		Service: d.Id(),
		Version: s.ActiveVersion.Number,
	})

	if err != nil {
		return fmt.Errorf("[ERR] Error looking up HTTPS logging endpoints for (%s), version (%v): %s", d.Id(), s.ActiveVersion.Number, err)
	}

	hll := flattenHTTPS(httpsList)

	if err := d.Set("httpslogging", hll); err != nil {
		log.Printf("[WARN] Error setting HTTPS logging endpoints for (%s): %s", d.Id(), err)
	}

	return nil
}

func createHTTPS(conn *gofastly.Client, i *gofastly.CreateHTTPSInput) error {
	_, err := conn.CreateHTTPS(i)
	if err != nil {
		return err
	}
	return nil
}

func deleteHTTPS(conn *gofastly.Client, i *gofastly.DeleteHTTPSInput) error {
	err := conn.DeleteHTTPS(i)

	if errRes, ok := err.(*gofastly.HTTPError); ok {
		if errRes.StatusCode != 404 {
			return err
		}
	} else if err != nil {
		return err
	}

	return nil
}

func flattenHTTPS(httpsList []*gofastly.HTTPS) []map[string]interface{} {
	var hsl []map[string]interface{}
	for _, hl := range httpsList {
		// Convert HTTP logging to a map for saving to state.
		nhl := map[string]interface{}{
			"name":                hl.Name,
			"response_condition":  hl.ResponseCondition,
			"format":              hl.Format,
			"url":                 hl.URL,
			"request_max_entries": hl.RequestMaxEntries,
			"request_max_bytes":   hl.RequestMaxBytes,
			"content_type":        hl.ContentType,
			"header_name":         hl.HeaderName,
			"header_value":        hl.HeaderValue,
			"method":              hl.Method,
			"json_format":         hl.JSONFormat,
			"placement":           hl.Placement,
			"tls_ca_cert":         hl.TLSCACert,
			"tls_client_cert":     hl.TLSClientCert,
			"tls_client_key":      hl.TLSClientKey,
			"tls_hostname":        hl.TLSHostname,
			"message_type":        hl.MessageType,
			"format_version":      hl.FormatVersion,
		}

		// prune any empty values that come from the default string value in structs
		for k, v := range nhl {
			if v == "" {
				delete(nhl, k)
			}
		}

		hsl = append(hsl, nhl)
	}

	return hsl
}

func buildCreateHTTPS(httpsMap interface{}, serviceID string, serviceVersion int) *gofastly.CreateHTTPSInput {
	df := httpsMap.(map[string]interface{})
	opts := gofastly.CreateHTTPSInput{
		Service:           serviceID,
		Version:           serviceVersion,
		Name:              df["name"].(string),
		URL:               df["url"].(string),
		RequestMaxEntries: uint(df["request_max_entries"].(int)),
		RequestMaxBytes:   uint(df["request_max_bytes"].(int)),
		ContentType:       df["content_type"].(string),
		HeaderName:        df["header_name"].(string),
		HeaderValue:       df["header_value"].(string),
		Method:            df["method"].(string),
		JSONFormat:        df["json_format"].(string),
		TLSCACert:         df["tls_ca_cert"].(string),
		TLSClientCert:     df["tls_client_cert"].(string),
		TLSClientKey:      df["tls_client_key"].(string),
		TLSHostname:       df["tls_hostname"].(string),
		Format:            df["format"].(string),
		FormatVersion:     uint(df["format_version"].(int)),
		MessageType:       df["message_type"].(string),
		Placement:         df["placement"].(string),
		ResponseCondition: df["response_condition"].(string),
	}

	return &opts
}

func buildDeleteHTTPS(httpsMap interface{}, serviceID string, serviceVersion int) *gofastly.DeleteHTTPSInput {
	df := httpsMap.(map[string]interface{})

	opts := gofastly.DeleteHTTPSInput{
		Service: serviceID,
		Version: serviceVersion,
		Name:    df["name"].(string),
	}

	return &opts
}
