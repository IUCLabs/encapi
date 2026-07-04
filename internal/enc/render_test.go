package enc

import (
	"reflect"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/IUCLabs/encapi/pkg/models"
)

func unmarshal(t *testing.T, b []byte) map[string]any {
	t.Helper()
	var m map[string]any
	if err := yaml.Unmarshal(b, &m); err != nil {
		t.Fatalf("unmarshal: %v\n%s", err, b)
	}
	return m
}

func TestFinalDropsTestingEnvironment(t *testing.T) {
	out, err := Final(models.Node{Certname: "h", Role: "roles::infra::storage::vault", Environment: "testing"}, models.Role{Name: "roles::infra::storage::vault"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	doc := unmarshal(t, out)
	if _, ok := doc["environment"]; ok {
		t.Error("environment must be omitted when testing")
	}
	classes, ok := doc["classes"].([]any)
	if !ok || len(classes) != 1 || classes[0] != "roles::infra::storage::vault" {
		t.Errorf("classes = %#v, want single-element list", doc["classes"])
	}
	params := doc["parameters"].(map[string]any)
	if params["enc_env"] != "testing" {
		t.Errorf("enc_env = %v, want testing", params["enc_env"])
	}
	encRole, _ := params["enc_role"].([]any)
	if len(encRole) != 1 || encRole[0] != "roles::infra::storage::vault" {
		t.Errorf("enc_role = %#v", params["enc_role"])
	}
}

func TestFinalKeepsNonTestingEnvironment(t *testing.T) {
	out, err := Final(models.Node{Certname: "h", Role: "roles::base", Environment: "production"}, models.Role{Name: "roles::base"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	doc := unmarshal(t, out)
	if doc["environment"] != "production" {
		t.Errorf("environment = %v, want production", doc["environment"])
	}
	if doc["parameters"].(map[string]any)["enc_env"] != "production" {
		t.Error("enc_env should equal environment")
	}
}

func TestParamPrecedence(t *testing.T) {
	// distro < role default < node param
	node := models.Node{Certname: "h", Role: "r", Environment: "production", Params: map[string]any{"shared": "node", "only_node": 1}}
	role := models.Role{Name: "r", DefaultParams: map[string]any{"shared": "role", "only_role": 2}}
	distro := map[string]any{"shared": "distro", "only_distro": 3}

	out, err := Final(node, role, distro)
	if err != nil {
		t.Fatal(err)
	}
	params := unmarshal(t, out)["parameters"].(map[string]any)
	if params["shared"] != "node" {
		t.Errorf("shared = %v, want node (node param wins)", params["shared"])
	}
	if params["only_role"] != 2 || params["only_distro"] != 3 || params["only_node"] != 1 {
		t.Errorf("missing merged params: %#v", params)
	}
}

func TestReservedParamsCannotBeOverridden(t *testing.T) {
	// A malicious/mistaken param must not shadow the computed enc_role/enc_env.
	node := models.Node{Certname: "h", Role: "roles::real", Environment: "production", Params: map[string]any{"enc_role": []string{"roles::fake"}, "enc_env": "hacked"}}
	out, err := Final(node, models.Role{Name: "roles::real"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	params := unmarshal(t, out)["parameters"].(map[string]any)
	if params["enc_env"] != "production" {
		t.Errorf("enc_env = %v, computed value must win", params["enc_env"])
	}
	encRole := params["enc_role"].([]any)
	if encRole[0] != "roles::real" {
		t.Errorf("enc_role = %#v, computed value must win", encRole)
	}
}

func TestCobblerShape(t *testing.T) {
	node := models.Node{Certname: "h", Role: "roles::infra::storage::vault", Environment: "testing"}
	role := models.Role{Name: "roles::infra::storage::vault", DefaultParams: map[string]any{"minio_pool": "pool1"}}
	out, err := Cobbler(node, role, nil)
	if err != nil {
		t.Fatal(err)
	}
	doc := unmarshal(t, out)
	// environment is ALWAYS present in cobbler-wire form, even for testing.
	if doc["environment"] != "testing" {
		t.Errorf("environment = %v, want testing (always present)", doc["environment"])
	}
	// classes is a MAP keyed by role name, whose value carries class params.
	classes, ok := doc["classes"].(map[string]any)
	if !ok {
		t.Fatalf("classes not a map: %#v", doc["classes"])
	}
	cp, ok := classes["roles::infra::storage::vault"].(map[string]any)
	if !ok {
		t.Fatalf("missing role key in classes: %#v", classes)
	}
	if cp["minio_pool"] != "pool1" {
		t.Errorf("class params = %#v, want minio_pool", cp)
	}
}

func TestMergeParamsNeverNil(t *testing.T) {
	if got := mergeParams(models.Node{}, models.Role{}, nil); !reflect.DeepEqual(got, map[string]any{}) {
		t.Errorf("mergeParams = %#v, want empty non-nil map", got)
	}
}
