package device

import (
	"strings"

	"github.com/google/uuid"

	"github.com/netbirdio/netbird/client/iface/configurer"
)

// instanceGUIDNamespace seeds the GUIDs derived from interface names, so the
// same name always yields the same GUID and no other software derives it.
var instanceGUIDNamespace = uuid.NewSHA1(uuid.NameSpaceOID, []byte("netbird.wintun.instance"))

// instanceGUIDString returns the Wintun adapter GUID for an interface that is
// not the default one, or "" for the default interface, which keeps its fixed
// GUID. Two adapters cannot share a GUID, so each instance derives its own.
func instanceGUIDString(ifaceName string) string {
	if ifaceName == "" || strings.EqualFold(ifaceName, configurer.WgInterfaceDefault) {
		return ""
	}
	id := uuid.NewSHA1(instanceGUIDNamespace, []byte(strings.ToLower(ifaceName)))
	return "{" + id.String() + "}"
}
