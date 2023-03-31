package kongstate

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/kong/go-kong/kong"
	"github.com/sirupsen/logrus"
	gatewayv1alpha2 "sigs.k8s.io/gateway-api/apis/v1alpha2"

	"github.com/kong/kubernetes-ingress-controller/v2/internal/annotations"
	"github.com/kong/kubernetes-ingress-controller/v2/internal/util"
	configurationv1 "github.com/kong/kubernetes-ingress-controller/v2/pkg/apis/configuration/v1"
)

// Route represents a Kong Route and holds a reference to the Ingress
// rule.
type Route struct {
	kong.Route

	Ingress util.K8sObjectInfo
	Plugins []kong.Plugin
}

var (
	validMethods = regexp.MustCompile(`\A[A-Z]+$`)

	// hostnames are complicated. shamelessly cribbed from https://stackoverflow.com/a/18494710
	// TODO if the Kong core adds support for wildcard SNI route match criteria, this should change.
	validSNIs  = regexp.MustCompile(`^([a-zA-Z0-9]+(-[a-zA-Z0-9]+)*)+(\.([a-zA-Z0-9]+(-[a-zA-Z0-9]+)*))*$`)
	validHosts = regexp.MustCompile(`^(\*\.)?([a-zA-Z0-9]+(-[a-zA-Z0-9]+)*)+(\.([a-zA-Z0-9]+(-[a-zA-Z0-9]+)*))*?(\.\*)?$`)
)

// normalizeProtocols prevents users from mismatching grpc/http.
func (r *Route) normalizeProtocols() {
	protocols := r.Protocols
	var http, grpc bool

	for _, protocol := range protocols {
		if strings.Contains(*protocol, "grpc") {
			grpc = true
		}
		if strings.Contains(*protocol, "http") {
			http = true
		}
		if !util.ValidateProtocol(*protocol) {
			http = true
		}
	}

	if grpc && http {
		r.Protocols = kong.StringSlice("http", "https")
	}
}

// useSSLProtocol updates the protocol of the route to either https or grpcs, or https and grpcs.
func (r *Route) useSSLProtocol() {
	var http, grpc bool
	var prots []*string

	for _, val := range r.Protocols {

		if strings.Contains(*val, "grpc") {
			grpc = true
		}

		if strings.Contains(*val, "http") {
			http = true
		}
	}

	if grpc {
		prots = append(prots, kong.String("grpcs"))
	}
	if http {
		prots = append(prots, kong.String("https"))
	}

	if !grpc && !http {
		prots = append(prots, kong.String("https"))
	}

	r.Protocols = prots
}

func (r *Route) overrideStripPath(anns map[string]string) {
	if r == nil {
		return
	}

	stripPathValue := annotations.ExtractStripPath(anns)
	if stripPathValue == "" {
		return
	}
	stripPathValue = strings.ToLower(stripPathValue)
	switch stripPathValue {
	case "true":
		r.StripPath = kong.Bool(true)
	case "false":
		r.StripPath = kong.Bool(false)
	default:
		return
	}
}

func (r *Route) overrideProtocols(anns map[string]string) {
	protocols := annotations.ExtractProtocolNames(anns)
	var prots []*string
	for _, prot := range protocols {
		if !util.ValidateProtocol(prot) {
			return
		}
		prots = append(prots, kong.String(prot))
	}

	r.Protocols = prots
}

func (r *Route) overrideHTTPSRedirectCode(anns map[string]string) {
	if annotations.HasForceSSLRedirectAnnotation(anns) {
		r.HTTPSRedirectStatusCode = kong.Int(302)
		r.useSSLProtocol()
	}

	code := annotations.ExtractHTTPSRedirectStatusCode(anns)
	if code == "" {
		return
	}
	statusCode, err := strconv.Atoi(code)
	if err != nil {
		return
	}
	if statusCode != 426 &&
		statusCode != 301 &&
		statusCode != 302 &&
		statusCode != 307 &&
		statusCode != 308 {
		return
	}

	r.HTTPSRedirectStatusCode = kong.Int(statusCode)
}

func (r *Route) overridePreserveHost(anns map[string]string) {
	preserveHostValue := annotations.ExtractPreserveHost(anns)
	if preserveHostValue == "" {
		return
	}
	preserveHostValue = strings.ToLower(preserveHostValue)
	switch preserveHostValue {
	case "true":
		r.PreserveHost = kong.Bool(true)
	case "false":
		r.PreserveHost = kong.Bool(false)
	default:
		return
	}
}

func (r *Route) overrideRegexPriority(anns map[string]string) {
	priority := annotations.ExtractRegexPriority(anns)
	if priority == "" {
		return
	}
	regexPriority, err := strconv.Atoi(priority)
	if err != nil {
		return
	}

	r.RegexPriority = kong.Int(regexPriority)
}

func (r *Route) overrideMethods(log logrus.FieldLogger, anns map[string]string) {
	annMethods := annotations.ExtractMethods(anns)
	if len(annMethods) == 0 {
		return
	}
	var methods []*string
	for _, method := range annMethods {
		sanitizedMethod := strings.TrimSpace(strings.ToUpper(method))
		if validMethods.MatchString(sanitizedMethod) {
			methods = append(methods, kong.String(sanitizedMethod))
		} else {
			// if any method is invalid (not an uppercase alpha string),
			// discard everything
			log.WithField("kongroute", r.Name).Errorf("invalid method: %v", method)
			return
		}
	}

	r.Methods = methods
}

func (r *Route) overrideSNIs(log logrus.FieldLogger, anns map[string]string) {
	var annSNIs []string
	var exists bool
	annSNIs, exists = annotations.ExtractSNIs(anns)
	// this is not a length check because we specifically want to provide a means
	// to set "no SNI criteria", by providing the annotation with an empty string value
	if !exists {
		return
	}
	var snis []*string
	for _, sni := range annSNIs {
		sanitizedSNI := strings.TrimSpace(sni)
		if validSNIs.MatchString(sanitizedSNI) {
			snis = append(snis, kong.String(sanitizedSNI))
		} else {
			// SNI is not a valid hostname
			log.WithField("kongroute", r.Name).Errorf("invalid SNI: %v", sni)
			return
		}
	}

	r.SNIs = snis
}

// overrideByAnnotation sets Route protocols via annotation.
func (r *Route) overrideByAnnotation(log logrus.FieldLogger) {
	r.overrideProtocols(r.Ingress.Annotations)
	r.overrideStripPath(r.Ingress.Annotations)
	r.overrideHTTPSRedirectCode(r.Ingress.Annotations)
	r.overridePreserveHost(r.Ingress.Annotations)
	r.overrideRegexPriority(r.Ingress.Annotations)
	r.overrideMethods(log, r.Ingress.Annotations)
	r.overrideSNIs(log, r.Ingress.Annotations)
	r.overrideRequestBuffering(log, r.Ingress.Annotations)
	r.overrideResponseBuffering(log, r.Ingress.Annotations)
	r.overrideHosts(log, r.Ingress.Annotations)
}

// override sets Route fields by KongIngress first, then by annotation.
func (r *Route) override(log logrus.FieldLogger, kongIngress *configurationv1.KongIngress) {
	if r == nil {
		return
	}

	// Check if we're trying to get KongIngress configuration based on an annotation
	// on Gateway API object (this would most likely happen for *Route objects but
	// log a warning for all other Gateway API objects as well since that also should
	// not happen) and if that's the case then skip it since those should not be
	// affected by said annotation.
	if gvk := r.Ingress.GroupVersionKind; gvk.Group == gatewayv1alpha2.GroupName && kongIngress != nil {
		log.WithFields(logrus.Fields{
			"resource_name":      r.Ingress.Name,
			"resource_namespace": r.Ingress.Namespace,
			"resource_kind":      gvk.Kind,
		}).Warn("KongIngress annotation is not allowed on Gateway API objects.")
		return
	}

	r.overrideByKongIngress(log, kongIngress)
	r.overrideByAnnotation(log)
	r.normalizeProtocols()
	for _, val := range r.Protocols {
		if *val == "grpc" || *val == "grpcs" {
			// grpc(s) doesn't accept strip_path
			r.StripPath = nil
			break
		}
	}
}

// overrideByKongIngress sets Route fields by KongIngress.
func (r *Route) overrideByKongIngress(log logrus.FieldLogger, kongIngress *configurationv1.KongIngress) {
	if kongIngress == nil || kongIngress.Route == nil {
		return
	}

	ir := kongIngress.Route
	if len(ir.Methods) != 0 {
		invalid := false
		var methods []*string
		for _, method := range ir.Methods {
			sanitizedMethod := strings.TrimSpace(strings.ToUpper(*method))
			if validMethods.MatchString(sanitizedMethod) {
				methods = append(methods, kong.String(sanitizedMethod))
			} else {
				// if any method is invalid (not an uppercase alpha string),
				// discard everything
				log.WithFields(logrus.Fields{
					"ingress_namespace": r.Ingress.Namespace,
					"ingress_name":      r.Ingress.Name,
				}).Errorf("ingress contains invalid method: '%v'", *method)
				invalid = true
			}
		}
		if !invalid {
			r.Methods = methods
		}
	}
	if len(ir.Headers) != 0 {
		r.Headers = ir.Headers
	}
	if len(ir.Protocols) != 0 {
		r.Protocols = protocolPointersToStringPointers(ir.Protocols)
	}
	if ir.RegexPriority != nil {
		r.RegexPriority = kong.Int(*ir.RegexPriority)
	}
	if ir.StripPath != nil {
		r.StripPath = kong.Bool(*ir.StripPath)
	}
	if ir.PreserveHost != nil {
		r.PreserveHost = kong.Bool(*ir.PreserveHost)
	}
	if ir.HTTPSRedirectStatusCode != nil {
		r.HTTPSRedirectStatusCode = kong.Int(*ir.HTTPSRedirectStatusCode)
	}
	if ir.PathHandling != nil {
		r.PathHandling = kong.String(*ir.PathHandling)
	}
	if len(ir.SNIs) != 0 {
		var SNIs []*string
		for _, unsanitizedSNI := range ir.SNIs {
			SNI := strings.TrimSpace(*unsanitizedSNI)
			if validSNIs.MatchString(SNI) {
				SNIs = append(SNIs, kong.String(SNI))
			} else {
				// SNI is not a valid hostname
				log.WithField("kongroute", r.Name).Errorf("invalid SNI: %v", unsanitizedSNI)
				return
			}
		}
		r.SNIs = SNIs
	}
	if ir.RequestBuffering != nil {
		r.RequestBuffering = kong.Bool(*ir.RequestBuffering)
	}
	if ir.ResponseBuffering != nil {
		r.ResponseBuffering = kong.Bool(*ir.ResponseBuffering)
	}
}

// overrideRequestBuffering ensures defaults for the request_buffering option.
func (r *Route) overrideRequestBuffering(log logrus.FieldLogger, anns map[string]string) {
	annotationValue, ok := annotations.ExtractRequestBuffering(anns)
	if !ok {
		// the annotation is not set, quit
		return
	}

	isEnabled, err := strconv.ParseBool(strings.ToLower(annotationValue))
	if err != nil {
		// the value provided is not a parseable boolean, quit
		log.WithField("kongroute", r.Name).Errorf("invalid request_buffering value: %s", err)
		return
	}

	r.RequestBuffering = kong.Bool(isEnabled)
}

// overrideResponseBuffering ensures defaults for the response_buffering option.
func (r *Route) overrideResponseBuffering(log logrus.FieldLogger, anns map[string]string) {
	annotationValue, ok := annotations.ExtractResponseBuffering(anns)
	if !ok {
		// the annotation is not set, quit
		return
	}

	isEnabled, err := strconv.ParseBool(strings.ToLower(annotationValue))
	if err != nil {
		// the value provided is not a parseable boolean, quit
		log.WithField("kongroute", r.Name).Errorf("invalid response_buffering value: %s", err)
		return
	}

	r.ResponseBuffering = kong.Bool(isEnabled)
}

// overrideHosts appends Host-Aliases to Hosts.
func (r *Route) overrideHosts(log logrus.FieldLogger, anns map[string]string) {
	var hosts []*string
	var annHostAliases []string
	var exists bool
	annHostAliases, exists = annotations.ExtractHostAliases(anns)
	if !exists {
		// the annotation is not set, quit
		return
	}

	// avoid allowing duplicate hosts or host-aliases from being added
	appendIfMissing := func(hosts []*string, sanitizedHost string) []*string {
		for _, uniqueHost := range hosts {
			if *uniqueHost == sanitizedHost {
				return hosts
			}
		}
		return append(hosts, kong.String(sanitizedHost))
	}

	// Merge hosts and host-aliases
	hosts = append(hosts, r.Hosts...)
	for _, hostAlias := range annHostAliases {
		sanitizedHost := strings.TrimSpace(hostAlias)
		if validHosts.MatchString(sanitizedHost) {
			hosts = appendIfMissing(hosts, sanitizedHost)
		} else {
			// Host Alias is not a valid hostname
			log.WithField("kongroute", r.Name).Errorf("invalid host: %v", hostAlias)
			return
		}
	}

	r.Hosts = hosts
}
