package secrets

import (
	"errors"
	"fmt"
	"io"
	"io/ioutil"

	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/client"
	kcmdutil "k8s.io/kubernetes/pkg/kubectl/cmd/util"

	cmdutil "github.com/openshift/origin/pkg/cmd/util"

	"github.com/spf13/cobra"
)

const (
	// CreateBasicAuthSecretRecommendedCommandName represents name of subcommand for `oc secrets` command
	CreateBasicAuthSecretRecommendedCommandName = "new-basicauth"

	createBasicAuthSecretLong = `
Create a new basic authentication secret

Basic authenticate secrets are used to authenticate against SCM servers.

When creating applications, you may have a SCM server that requires basic authentication - username, password.
In order for the nodes to clone source code on your behalf, they have to have the credentials. You can provide 
this information by creating a 'basicauth' secret and attaching it to your service account.`

	createBasicAuthSecretExample = `  // If your basic authentication method requires only username and password or token, add it by using:
  $ %[1]s SECRET --username=USERNAME --password=PASSWORD

  // If your basic authentication method requires also CA certificate, add it by using:
  $ %[1]s SECRET --username=USERNAME --password=PASSWORD --ca-cert=FILENAME

  // If you do already have a .gitconfig file needed for authentication, you can create a gitconfig secret by using:
  $ %[2]s SECRET path/to/.gitconfig`
)

// CreateBasicAuthSecretOptions holds the credential needed to authenticate against SCM servers.
type CreateBasicAuthSecretOptions struct {
	SecretName      string
	Username        string
	Password        string
	CertificatePath string
	GitConfigPath   string

	PromptForPassword bool

	Reader io.Reader
	Out    io.Writer

	SecretsInterface client.SecretsInterface
}

// NewCmdCreateBasicAuthSecret implements the OpenShift cli secrets new-basicauth subcommand
func NewCmdCreateBasicAuthSecret(name, fullName string, f *kcmdutil.Factory, reader io.Reader, out io.Writer, newSecretFullName, ocEditFullName string) *cobra.Command {
	o := &CreateBasicAuthSecretOptions{
		Out:    out,
		Reader: reader,
	}

	cmd := &cobra.Command{
		Use:     fmt.Sprintf("%s SECRET_NAME --username=USERNAME --password=PASSWORD [--ca-cert=FILENAME --gitconfig=FILENAME]", name),
		Short:   "Create a new secret for basic authentication",
		Long:    createBasicAuthSecretLong,
		Example: fmt.Sprintf(createBasicAuthSecretExample, fullName, newSecretFullName, ocEditFullName),
		Run: func(c *cobra.Command, args []string) {
			if err := o.Complete(f, args); err != nil {
				kcmdutil.CheckErr(kcmdutil.UsageError(c, err.Error()))
			}

			if err := o.Validate(); err != nil {
				kcmdutil.CheckErr(kcmdutil.UsageError(c, err.Error()))
			}

			if len(kcmdutil.GetFlagString(c, "output")) != 0 {
				secret, err := o.NewBasicAuthSecret()
				kcmdutil.CheckErr(err)

				kcmdutil.CheckErr(f.PrintObject(c, secret, out))
				return
			}

			if err := o.CreateBasicAuthSecret(); err != nil {
				kcmdutil.CheckErr(err)
			}
		},
	}

	cmd.Flags().StringVar(&o.Username, "username", "", "Username for Git authentication")
	cmd.Flags().StringVar(&o.Password, "password", "", "Password or token for Git authentication")
	cmd.Flags().StringVar(&o.CertificatePath, "ca-cert", "", "Path to a certificate file")
	cmd.Flags().StringVar(&o.GitConfigPath, "gitconfig", "", "Path to a .gitconfig file")
	cmd.Flags().BoolVarP(&o.PromptForPassword, "prompt", "", false, "Prompt for password or token")

	// autocompletion hints
	cmd.MarkFlagFilename("ca-cert")
	cmd.MarkFlagFilename("gitconfig")

	kcmdutil.AddPrinterFlags(cmd)

	return cmd
}

// CreateBasicAuthSecret saves created Secret structure and prints the secret name to the output on success.
func (o *CreateBasicAuthSecretOptions) CreateBasicAuthSecret() error {
	secret, err := o.NewBasicAuthSecret()
	if err != nil {
		return err
	}

	if _, err := o.SecretsInterface.Create(secret); err != nil {
		return err
	}

	fmt.Fprintf(o.GetOut(), "secret/%s\n", secret.Name)
	return nil
}

// NewBasicAuthSecret builds up the Secret structure containing secret name, type and data structure
// containing desired credentials.
func (o *CreateBasicAuthSecretOptions) NewBasicAuthSecret() (*api.Secret, error) {
	secret := &api.Secret{}
	secret.Name = o.SecretName
	secret.Type = api.SecretTypeOpaque
	secret.Data = map[string][]byte{}

	if len(o.Username) != 0 {
		secret.Data[SourceUsername] = []byte(o.Username)
	}

	if len(o.Password) != 0 {
		secret.Data[SourcePassword] = []byte(o.Password)
	}

	if len(o.CertificatePath) != 0 {
		caContent, err := ioutil.ReadFile(o.CertificatePath)
		if err != nil {
			return nil, err
		}
		secret.Data[SourceCertificate] = caContent
	}

	if len(o.GitConfigPath) != 0 {
		gitConfig, err := ioutil.ReadFile(o.GitConfigPath)
		if err != nil {
			return nil, err
		}
		secret.Data[SourceGitConfig] = gitConfig
	}

	return secret, nil
}

// Complete fills CreateBasicAuthSecretOptions fields with data and checks for mutual exclusivity
// between flags from different option groups.
func (o *CreateBasicAuthSecretOptions) Complete(f *kcmdutil.Factory, args []string) error {
	if len(args) != 1 {
		return errors.New("must have exactly one argument: secret name")
	}
	o.SecretName = args[0]

	if f != nil {
		client, err := f.Client()
		if err != nil {
			return err
		}
		namespace, _, err := f.DefaultNamespace()
		if err != nil {
			return err
		}
		o.SecretsInterface = client.Secrets(namespace)
	}

	return nil
}

// Validate check if all necessary fields from CreateBasicAuthSecretOptions are present.
func (o CreateBasicAuthSecretOptions) Validate() error {
	if len(o.SecretName) == 0 {
		return errors.New("basic authentication secret name must be present")
	}

	if o.PromptForPassword {
		if len(o.Password) != 0 {
			return errors.New("must provide either --prompt or --password flag")
		}
		if cmdutil.IsTerminal(o.Reader) {
			o.Password = cmdutil.PromptForPasswordString(o.Reader, o.Out, "Password: ")
			if len(o.Password) == 0 {
				return errors.New("password must be provided")
			}
		} else {
			return errors.New("provided reader is not a terminal")
		}
	} else {
		if len(o.Username) == 0 && len(o.Password) == 0 && len(o.CertificatePath) == 0 {
			return errors.New("must provide basic authentication credentials")
		}
	}

	return nil
}

// GetOut check if the CreateBasicAuthSecretOptions Out Writer is set. Returns it if the Writer
// is present, if not returns Writer on which all Write calls succeed without doing anything.
func (o CreateBasicAuthSecretOptions) GetOut() io.Writer {
	if o.Out == nil {
		return ioutil.Discard
	}

	return o.Out
}
