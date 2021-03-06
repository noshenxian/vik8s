package volumes

import (
	"github.com/ihaiker/vik8s/libs/utils"
	"github.com/ihaiker/vik8s/reduce/config"
	v1 "k8s.io/api/core/v1"
)

func secretParse(v *v1.Volume, source string, args []string, body config.Directives) {
	v.Secret = &v1.SecretVolumeSource{
		SecretName: source,
	}

	if d := utils.Index(args, 0); d != "" {
		v.Secret.DefaultMode = utils.Int32(d, 8)
	}
	if d := body.Remove("defaultModule"); d != nil {
		v.Secret.DefaultMode = utils.Int32(d.Args[0], 8)
	}

	setBody := func(items config.Directives) {
		if len(items) > 0 {
			for _, body := range items {
				name, mode := utils.Split2(body.Name, ":")
				path := utils.Default(body.Args, 1, name)
				kp := v1.KeyToPath{Key: name, Path: path}
				if mode != "" {
					kp.Mode = utils.Int32(mode, 8)
				}
				v.Secret.Items = append(v.Secret.Items, kp)
			}
		}
	}

	if items := body.Remove("items"); items != nil {
		setBody(items.Body)
	}
	setBody(body)
}
