package validate

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
)

// PortMappings returns a string validator that checks the comma-separated
// mappings use Docker's short port syntax: an optional IP and host port, a
// container port, an optional tcp/udp/sctp suffix, and optional ranges.
func PortMappings() validator.String {
	return portMappingsValidator{}
}

type portMappingsValidator struct{}

func (v portMappingsValidator) Description(_ context.Context) string {
	return "must be comma-separated Docker port mappings such as \"8080:80\", \"8080:80/udp\", \"127.0.0.1:8080:80\" or \"8000-8010:8000-8010\" with ports between 1 and 65535"
}

func (v portMappingsValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v portMappingsValidator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}

	value := req.ConfigValue.ValueString()
	if value == "" {
		resp.Diagnostics.AddAttributeError(req.Path, "Invalid Port Mapping",
			"port mappings must not be empty; omit the attribute instead of setting it to an empty string")
		return
	}

	for _, pair := range strings.Split(value, ",") {
		if err := validatePortMapping(strings.TrimSpace(pair)); err != nil {
			resp.Diagnostics.AddAttributeError(req.Path, "Invalid Port Mapping", err.Error())
			return
		}
	}
}

// validatePortMapping validates one mapping of the form
// [[ip:]host_port:]container_port[/tcp|/udp|/sctp], where ports may be ranges.
func validatePortMapping(pair string) error {
	if pair == "" {
		return fmt.Errorf("port mappings must not contain empty entries")
	}

	if i := strings.LastIndex(pair, "/"); i >= 0 {
		switch pair[i+1:] {
		case "tcp", "udp", "sctp":
		default:
			return fmt.Errorf("protocol %q must be one of tcp, udp or sctp", pair[i+1:])
		}
		pair = pair[:i]
	}

	if ip, rest, ok := splitLeadingIP(pair); ok {
		if !validIP(ip) {
			return fmt.Errorf("invalid IP address %q", ip)
		}
		pair = rest
	}

	host, container, ok := strings.Cut(pair, ":")
	if !ok {
		return fmt.Errorf("expected host:container format with optional IP and protocol, got %q", pair)
	}

	hostPorts, err := parsePortCount(host, true)
	if err != nil {
		return err
	}
	containerPorts, err := parsePortCount(container, false)
	if err != nil {
		return err
	}
	if hostPorts > 0 && hostPorts != containerPorts {
		return fmt.Errorf("host range %q must expose the same number of ports as container range %q", strings.TrimSpace(host), strings.TrimSpace(container))
	}
	return nil
}

// splitLeadingIP separates an optional host IP prefix from the rest of the
// mapping. IPv6 addresses must be bracketed, as in "[::1]:8080:80".
func splitLeadingIP(s string) (ip, rest string, ok bool) {
	if strings.HasPrefix(s, "[") {
		end := strings.Index(s, "]")
		if end < 0 || end+1 >= len(s) || s[end+1] != ':' {
			return "", s, false
		}
		return s[:end+1], s[end+2:], true
	}
	head, rest, found := strings.Cut(s, ":")
	if !found || net.ParseIP(head) == nil {
		return "", s, false
	}
	return head, rest, true
}

func validIP(ip string) bool {
	if strings.HasPrefix(ip, "[") && strings.HasSuffix(ip, "]") {
		inner := ip[1 : len(ip)-1]
		return strings.Contains(inner, ":") && net.ParseIP(inner) != nil
	}
	parsed := net.ParseIP(ip)
	return parsed != nil && parsed.To4() != nil
}

// parsePortCount returns the number of host ports in a port token, or 0 when
// the host port is omitted (Docker then picks a random host port).
func parsePortCount(token string, allowEmpty bool) (int, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		if allowEmpty {
			return 0, nil
		}
		return 0, fmt.Errorf("container port is required")
	}

	start, end, isRange := strings.Cut(token, "-")
	low, err := parsePort(start)
	if err != nil {
		return 0, err
	}
	if !isRange {
		return 1, nil
	}
	high, err := parsePort(end)
	if err != nil {
		return 0, err
	}
	if low > high {
		return 0, fmt.Errorf("port range %q must be ascending", token)
	}
	return high - low + 1, nil
}

func parsePort(s string) (int, error) {
	s = strings.TrimSpace(s)
	port, err := strconv.Atoi(s)
	if err != nil || port < 1 || port > 65535 {
		return 0, fmt.Errorf("port %q must be an integer between 1 and 65535", s)
	}
	return port, nil
}
