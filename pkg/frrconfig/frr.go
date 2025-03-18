package frrconfig

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"text/template"
	"time"
)

// GlobalConfig structure
type GlobalConfig struct {
	ASN         uint16
	LogLevel    string
	Routers     []RouterConfig
	VRFs        []VRFConfig
	Interfaces  []InterfaceConfig
	PrefixLists []PrefixListConfig
	RouteMaps   []RouteMapConfig
}

// InterfaceConfig structure
type InterfaceConfig struct {
	Name string
	VRF  string
	IP   string
}

// PrefixListConfig structure
type PrefixListConfig struct {
	Name   string
	Seq    int
	Permit bool
	Prefix string
}

// RouteMapConfig structure
type RouteMapConfig struct {
	Name       string
	Permit     bool
	Seq        int
	PrefixList string
	MatchType  string
}

// StaticRouteConfig structure
type StaticRouteConfig struct {
	VRF         string
	Destination string
	NextHop     string
}

// VRFConfig structure
type VRFConfig struct {
	Name         string
	VNI          uint32
	StaticRoutes []StaticRouteConfig
}

// RouterConfig structure
type RouterConfig struct {
	VRF      string
	RouterID string
	Peers    []NeighborConfig
}

// NeighborConfig structure
type NeighborConfig struct {
	PeerIP      string
	PeerASN     string
	Password    string
	Description string
	RouteMapIn  string
	RouteMapOut string
}

/*const frrConfigTemplate = `
frr defaults traditional
hostname frr-k8s
log syslog {{.LogLevel}}

{{- range .Interfaces }}
interface {{.Name}}{{ if .VRF }} vrf {{.VRF}}{{end}}
 ip address {{.IP}}
exit
{{- end }}

{{- range .VRFs }}
vrf {{.Name}}
 vni {{.VNI}}
exit
{{- end }}

{{- range .Routers }}
router bgp {{$.ASN}}{{ if .VRF }} vrf {{.VRF}}
 bgp router-id {{.RouterID}}
 bgp log-neighbor-changes
 bgp graceful-restart
 no bgp ebgp-requires-policy
 no bgp network import-check
 no bgp default ipv4-unicast

{{- range .Peers }}
 neighbor {{.PeerIP}} remote-as {{.PeerASN}}
{{- if .Password }}
 neighbor {{.PeerIP}} password {{.Password}}
{{- end }}
{{- if .Description }}
 neighbor {{.PeerIP}} description "{{.Description}}"
{{- end }}
{{- end }}

 address-family ipv4 unicast
{{- range .Peers }}
  neighbor {{.PeerIP}} activate
{{- end }}
 exit-address-family

exit
{{- end }}
`*/

/*const frrConfigTemplate = `
frr defaults traditional
hostname frr-k8s
log syslog {{.LogLevel}}

{{- range .Interfaces }}
interface {{.Name}}
 vrf {{.VRF}}
 ip address {{.IP}}
exit
{{- end }}

{{- range .Routers }}
router bgp {{$.ASN}} vrf {{.VRF}}
 bgp router-id {{.RouterID}}
 bgp log-neighbor-changes
 bgp graceful-restart
 no bgp ebgp-requires-policy
 no bgp network import-check
 no bgp default ipv4-unicast

{{- range .Peers }}
 neighbor {{.PeerIP}} remote-as {{.PeerASN}}
{{- if .Password }}
 neighbor {{.PeerIP}} password {{.Password}}
{{- end }}
{{- if .Description }}
 neighbor {{.PeerIP}} description "{{.Description}}"
{{- end }}
{{- end }}

 address-family ipv4 unicast
{{- range .Peers }}
  neighbor {{.PeerIP}} activate
{{- end }}
 exit-address-family

exit
{{- end }}
`*/

const frrConfigTemplate = `
frr defaults traditional
hostname frr-k8s
log syslog {{.LogLevel}}

!{{- range $v:= .VRFs }}
vrf {{$v.Name}}{{- range $v.StaticRoutes }}
 ip route {{.Destination}} {{.NextHop}}{{- end }}
exit-vrf{{- end }}

{{- range .PrefixLists }}
ip prefix-list {{.Name}} seq {{.Seq}} {{if .Permit}}permit{{else}}deny{{end}} {{.Prefix}}
{{- end }}

{{- range .RouteMaps }}
route-map {{.Name}} {{if .Permit}}permit{{else}}deny{{end}} {{.Seq}}
 match ip address {{.PrefixList}}
 exit
{{- end }}

{{- range .Routers }}
router bgp {{$.ASN}} vrf {{.VRF}}
 bgp router-id {{.RouterID}}
 bgp log-neighbor-changes
 bgp graceful-restart
 no bgp ebgp-requires-policy
 no bgp network import-check
 no bgp default ipv4-unicast

{{- range .Peers }}
 neighbor {{.PeerIP}} remote-as {{.PeerASN}}
{{- if .Password }}
 neighbor {{.PeerIP}} password {{.Password}}
{{- end }}
{{- if .Description }}
 neighbor {{.PeerIP}} description "{{.Description}}"
{{- end }}
{{- if .RouteMapIn }}
 neighbor {{.PeerIP}} route-map {{.RouteMapIn}} in
{{- end }}
{{- if .RouteMapOut }}
 neighbor {{.PeerIP}} route-map {{.RouteMapOut}} out
{{- end }}
{{- end }}

 address-family ipv4 unicast
{{- range .Peers }}
  neighbor {{.PeerIP}} activate
{{- end }}
 exit-address-family

exit
{{- end }}
`

// updateVRFs ensures the VRFs are properly set up at the OS level
func updateVRFs(vrfs []VRFConfig) error {
	for _, vrf := range vrfs {
		fmt.Printf("Configuring VRF: %s with VNI: %d\n", vrf.Name, vrf.VNI)

		// Check if the VRF already exists
		checkCmd := exec.Command("ip", "link", "show", "type", "vrf")
		output, _ := checkCmd.CombinedOutput()

		if !strings.Contains(string(output), vrf.Name) {
			// Create the VRF if it doesn't exist
			fmt.Printf("Creating VRF %s\n", vrf.Name)
			cmd := exec.Command("ip", "link", "add", vrf.Name, "type", "vrf", "table", fmt.Sprintf("%d", vrf.VNI))
			err := cmd.Run()
			if err != nil {
				fmt.Printf("Error creating VRF %s: %v\n", vrf.Name, err)
				return err
			}
		}

		// Ensure the VRF is up
		cmd := exec.Command("ip", "link", "set", vrf.Name, "up")
		err := cmd.Run()
		if err != nil {
			fmt.Printf("Error bringing up VRF %s: %v\n", vrf.Name, err)
			return err
		}
	}
	return nil
}

func generateFRRConfig(config GlobalConfig, outputPath string) error {
	// Parse the template
	tmpl, err := template.New("frrConfig").Parse(frrConfigTemplate)
	if err != nil {
		return err
	}

	// Create or overwrite the configuration file
	file, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer file.Close()

	// This is a lifehack since vtysh.conf is required for FRR to start but its not present
	vtysh, err := os.Create("/etc/frr/vtysh.conf")
	if err != nil {
		return err
	}
	defer vtysh.Close()

	// Execute template and write to file
	err = tmpl.Execute(file, config)
	if err != nil {
		return err
	}

	fmt.Println("FRR configuration generated successfully at", outputPath)
	return nil
}

func reloadFRR() error {
	fmt.Println("Reloading FRR configuration...")
	cmd := exec.Command("/usr/lib/frr/frr-reload.py", "--reload", "--overwrite", "/etc/frr/frr.conf")
	output, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Println("Failed to reload FRR:", err, string(output))
		return err
	}
	fmt.Println("FRR successfully reloaded.")
	return nil
}

func main() {
	// Sample configuration
	globalConfig := GlobalConfig{
		ASN:      64512,
		LogLevel: "debug",
		Routers: []RouterConfig{
			{
				VRF:      "hedge",
				RouterID: "192.168.1.1",
				Peers: []NeighborConfig{
					{PeerIP: "192.168.1.2", PeerASN: "64513", Description: "hedge's friend"},
					{PeerIP: "192.168.1.3", PeerASN: "64514", Password: "nothedge"},
				},
			},
			{
				VRF:      "hog",
				RouterID: "192.168.2.1",
				Peers: []NeighborConfig{
					{PeerIP: "192.168.2.2", PeerASN: "64515", Description: "hog's friend", RouteMapIn: "test", RouteMapOut: "test"},
				},
			},
		},
		VRFs: []VRFConfig{
			{Name: "hedge", VNI: 100,
				StaticRoutes: []StaticRouteConfig{
					{VRF: "hedge", Destination: "0.0.0.0/0", NextHop: "192.168.1.1"},
				}},
			{Name: "hog", VNI: 200},
		},
		Interfaces: []InterfaceConfig{
			{Name: "eth0", VRF: "hedge", IP: "192.168.1.1/24"},
		},
		PrefixLists: []PrefixListConfig{
			{Name: "test", Seq: 10, Permit: true, Prefix: "10.10.0.0/16"},
		},
		RouteMaps: []RouteMapConfig{
			{Name: "test", Permit: true, Seq: 10, PrefixList: "test", MatchType: "ip"},
		},
	}

	// Generate FRR config and reload FRR periodically
	for {
		// Update VRFs
		err := updateVRFs(globalConfig.VRFs)
		if err != nil {
			fmt.Println("Error updating VRFs:", err)
		}

		err = generateFRRConfig(globalConfig, "/etc/frr/frr.conf")
		if err != nil {
			fmt.Println("Error generating FRR config:", err)
		} else {
			err = reloadFRR()
			if err != nil {
				fmt.Println("Error reloading FRR:", err)
			}
		}
		time.Sleep(30 * time.Second) // Refresh config every 30s
	}
}
