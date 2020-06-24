package cephfs

import (
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"github.com/ihaiker/vik8s/install/tools"
	"github.com/ihaiker/vik8s/libs/ssh"
	"k8s.io/apimachinery/pkg/util/rand"
	"strings"
	"time"
)

type Secrets struct {
	CephConf           string //etc/ceph/ceph.conf
	ClientAdminKey     string
	ClientAdminKeyring string //etc/ceph/ceph.client.admin.keyring

	MonKeyring string //etc/ceph/ceph.mon.keyring

	OSDBootstrapKeyring string //var/lib/ceph/bootstrap-osd/ceph.keyring
	MDSBootstrapKeyring string //var/lib/ceph/bootstrap-mds/ceph.keyring
	RGWBootstrapKeyring string //var/lib/ceph/bootstrap-rgw/ceph.keyring
	RBDBootstrapKeyring string //var/lib/ceph/bootstrap-rbd/ceph.keyring
}

var templateFuns = map[string]interface{}{
	"nodejoin": func(nodes ssh.Nodes, name string) string {
		out := make([]string, 0)
		for _, node := range nodes {
			switch name {
			case "name":
				out = append(out, node.Hostname)
			case "host":
				out = append(out, node.Host)
			case "monitors":
				out = append(out, fmt.Sprintf("%s:6789", node.Host))
			}
		}
		return strings.Join(out, ",")
	},
}

func (c *CephFS) generateSecrets(nodes ssh.Nodes) {
	/*
		secretPath := tools.Join("storage/ceph/secrets.json")
		_ = os.MkdirAll(filepath.Dir(secretPath), os.ModePerm)

		if utils.Exists(secretPath) {
			if data, err := ioutil.ReadFile(secretPath); err != nil {
				fmt.Printf("read %s error: %s \n", secretPath, err.Error())
			} else if err := json.Unmarshal(data, &c.Secrets); err != nil {
				fmt.Printf("read %s error: %s \n", secretPath, err.Error())
			} else {
				return
			}
		}
	*/

	c.genAllBootstrapKeyrings()
	c.genCombinedConf()

	/*
		data, _ := json.Marshal(&c.Secrets)
		if err := ioutil.WriteFile(secretPath, data, 0666); err != nil {
			fmt.Printf("write %s error: %s \n", secretPath, err.Error())
		}
	*/
}

func (c *CephFS) scpKeyrings(node *ssh.Node) {
	node.MustScpContent([]byte(c.Secrets.CephConf), "/etc/ceph/ceph.conf")
	node.MustScpContent([]byte(c.Secrets.ClientAdminKeyring), "/etc/ceph/ceph.client.admin.keyring")
	node.MustScpContent([]byte(c.Secrets.MonKeyring), "/etc/ceph/ceph.mon.keyring")

	node.MustScpContent([]byte(c.Secrets.OSDBootstrapKeyring), "/var/lib/ceph/bootstrap-osd/ceph.keyring")
	node.MustScpContent([]byte(c.Secrets.MDSBootstrapKeyring), "/var/lib/ceph/bootstrap-mds/ceph.keyring")
	node.MustScpContent([]byte(c.Secrets.RGWBootstrapKeyring), "/var/lib/ceph/bootstrap-rgw/ceph.keyring")
	node.MustScpContent([]byte(c.Secrets.RBDBootstrapKeyring), "/var/lib/ceph/bootstrap-rbd/ceph.keyring")
}

func (c *CephFS) genCombinedConf() {
	c.Secrets.CephConf = string(tools.MustAssert("yaml/storage/ceph/ceph.conf", c, templateFuns))

	//ceph.client.admin.keyring
	c.Secrets.ClientAdminKey = randomKey()
	c.Secrets.ClientAdminKeyring = fmt.Sprintf(`
[client.admin]
  key = %s
  auid = 0
  caps mds = "allow *"
  caps mon = "allow *"
  caps osd = "allow *"
  caps mgr = "allow *"
`, c.Secrets.ClientAdminKey)

	//ceph.mon.keyring
	c.Secrets.MonKeyring = fmt.Sprintf(`
[mon.]
key = %s
caps mon = "allow *"
`, c.Secrets.ClientAdminKey)

	//import
	c.Secrets.MonKeyring += "\n" + c.Secrets.ClientAdminKeyring

	c.Secrets.MonKeyring += "\n" + c.Secrets.OSDBootstrapKeyring
	c.Secrets.MonKeyring += "\n" + c.Secrets.MDSBootstrapKeyring
	c.Secrets.MonKeyring += "\n" + c.Secrets.RGWBootstrapKeyring
	c.Secrets.MonKeyring += "\n" + c.Secrets.RBDBootstrapKeyring
}

func randomKey() string {
	outs := make([]byte, 28)
	binary.LittleEndian.PutUint16(outs[0:2], 1)                         // le16 type: CEPH_CRYPTO_AES
	binary.LittleEndian.PutUint32(outs[2:6], uint32(time.Now().Unix())) // le32 created: seconds
	binary.LittleEndian.PutUint32(outs[6:10], uint32(0))                // le32 created: nanoseconds,
	binary.LittleEndian.PutUint16(outs[10:12], 16)                      // le16: len(key)
	copy(outs[12:], []byte(rand.String(16)))
	return base64.StdEncoding.EncodeToString(outs)
}

func (c *CephFS) genBootstrapKeyring(service string) string {
	return fmt.Sprintf(`
[client.bootstrap-%s]
  key = %s
  caps mon = "allow profile bootstrap-%s"
`, service, randomKey(), service)
}

func (c *CephFS) genAllBootstrapKeyrings() {
	c.Secrets.OSDBootstrapKeyring = c.genBootstrapKeyring("osd")
	c.Secrets.MDSBootstrapKeyring = c.genBootstrapKeyring("mds")
	c.Secrets.RGWBootstrapKeyring = c.genBootstrapKeyring("rgw")
	c.Secrets.RBDBootstrapKeyring = c.genBootstrapKeyring("rbd")
}
