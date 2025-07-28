package main

import (
	"fmt"

	"github.com/Golansami125/kiempossible/pkg/auth_handling"
)

func main() {
	banner := `
	 _  _____ ___ __  __ ___           _ _    _     
	| |/ /_ _| __|  \/  | _ \___ _____(_) |__| |___ 
	| ' < | || _|| |\/| |  _/ _ (_-<_-< | '_ \ / -_)
	|_|\_\___|___|_|  |_|_| \___/__/__/_|_.__/_\___|
                                                                            
`
	fmt.Println(banner)
	credPath, _, _ := auth_handling.Authenticator()
	Collect()
	if credPath.ShouldAdvise {
		Advise()
	}
}

// Imporvements - add GCP workload identity for ALL cases

// Access entries - add support for role bindings (and not just CRB)

// Potential additions:
// - automated remediation/suggest new role/binding based on usage etc
// - historical trends, reporting etc
// - policy-as-code integration, for example OPA for enforcing
// - add support for rancher/openshift/ibm cloud/oci
