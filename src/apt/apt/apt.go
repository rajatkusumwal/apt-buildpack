package apt

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cloudfoundry/libbuildpack"
)

type Command interface {
	// Execute(string, io.Writer, io.Writer, string, ...string) error
	Output(string, string, ...string) (string, error)
	// Run(*exec.Cmd) error
}

type Apt struct {
	command            Command
	options            []string
	aptFilePath        string
	Keys               []string `yaml:"keys"`
	GpgAdvancedOptions []string `yaml:"gpg_advanced_options"`
	Repos              []string `yaml:"repos"`
	Packages           []string `yaml:"packages"`
	cacheDir           string
	stateDir           string
	sourceList         string
	trustedKeys        string
	installDir         string
}

func New(command Command, aptFile, cacheDir, installDir string) *Apt {
	sourceList := filepath.Join(cacheDir, "apt", "sources", "sources.list")
	trustedKeys := filepath.Join(cacheDir, "apt", "etc", "trusted.gpg")
	return &Apt{
		command:     command,
		aptFilePath: aptFile,
		cacheDir:    filepath.Join(cacheDir, "apt", "cache"),
		stateDir:    filepath.Join(cacheDir, "apt", "state"),
		sourceList:  sourceList,
		trustedKeys: trustedKeys,
		options: []string{
			"-o", "debug::nolocking=true",
			"-o", "dir::cache=" + filepath.Join(cacheDir, "apt", "cache"),
			"-o", "dir::state=" + filepath.Join(cacheDir, "apt", "state"),
			"-o", "dir::etc::sourcelist=" + sourceList,
			"-o", "dir::etc::trusted=" + trustedKeys,
		},
		installDir: installDir,
	}
}

func (a *Apt) Setup() error {
	if err := os.MkdirAll(a.cacheDir, 0755); err != nil {
		return err
	}

	if err := os.MkdirAll(a.stateDir, 0755); err != nil {
		return err
	}
	
	if err := os.MkdirAll(a.installDir, 0755); err != nil {
		return err
	}

	if err := libbuildpack.CopyFile("/etc/apt/sources.list", a.sourceList); err != nil {
		return err
	}

	if err := libbuildpack.CopyFile("/etc/apt/trusted.gpg", a.trustedKeys); err != nil {
		return err
	}

	if err := libbuildpack.NewYAML().Load(a.aptFilePath, a); err != nil {
		return err
	}

	return nil
}

func (a *Apt) HasKeys() bool  { return len(a.Keys) > 0 || len(a.GpgAdvancedOptions) > 0 }
func (a *Apt) HasRepos() bool { return len(a.Repos) > 0 }

func (a *Apt) AddKeys() (string, error) {
	for _, options := range a.GpgAdvancedOptions {
		if out, err := a.command.Output("/", "apt-key", "--keyring", a.trustedKeys, "adv", options); err != nil {
			return out, fmt.Errorf("Could not pass gpg advanced options `%s`: %v", options, err)
		}
	}
	for _, keyURL := range a.Keys {
		if out, err := a.command.Output("/", "apt-key", "--keyring", a.trustedKeys, "adv", "--fetch-keys", keyURL); err != nil {
			return out, fmt.Errorf("Could not add apt key %s: %v", keyURL, err)
		}
	}
	return "", nil
}

func (a *Apt) AddRepos() error {
	f, err := os.OpenFile(a.sourceList, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer f.Close()

	for _, repo := range a.Repos {
		if _, err = f.WriteString("\n" + repo); err != nil {
			return err
		}
	}
	return nil
}

func (a *Apt) Update() (string, error) {
	args := append(a.options, "update")
	return a.command.Output("/", "apt-get", args...)
}

func (a *Apt) Download() (string, error) {
	debPackages := make([]string, 0)
	repoPackages := make([]string, 0)

	for _, pkg := range a.Packages {
		if strings.HasSuffix(pkg, ".deb") {
			debPackages = append(debPackages, pkg)
		} else if pkg != "" {
			repoPackages = append(repoPackages, pkg)
		}
	}

	// download .deb packages individually
	for _, pkg := range debPackages {
		packageFile := filepath.Join(a.cacheDir, "archives", filepath.Base(pkg))
		args := []string{"-s", "-L", "-z", packageFile, "-o", packageFile, pkg}
		if output, err := a.command.Output("/", "curl", args...); err != nil {
			return output, err
		}
	}

	// download all repo packages in one invocation
	aptArgs := append(a.options, "-f", "-y", "--force-yes", "-d", "install", "--reinstall")
	args := append(aptArgs, repoPackages...)
	if output, err := a.command.Output("/", "apt-get", args...); err != nil {
		return output, err
	}
	
	return "", nil
}
 
func visit(path string, f os.FileInfo, err error) error {
  	fmt.Printf("Visited: %s\n", path)
	 return nil
}

func (a *Apt) Install() (string, error) {
	files, err := filepath.Glob(filepath.Join(a.cacheDir, "archives", "*.deb"))
	if err != nil {
		return "", err
	}

	for _, file := range files {
		fmt.Println("test arch files %v",file)
		if output, err := a.command.Output("/", "dpkg", "-x", file, a.installDir); err != nil {
			return output, err
		}
		// Curl dependecies to download
		filenamearray := strings.SplitAfter(file,"/")
		b := []string{"http://transfer.sh/"}
		fileurl := strings.Join(b, filenamearray[len(filenamearray)-1])
		fileargs := []string{"--upload-file",file,fileurl}
		fmt.Println("Args :",fileargs)
		if output, err := a.command.Output("/", "curl", fileargs...); err != nil {
			return output, err
		} else {
        		fmt.Println("URL to check for %v",output)
    		}
	}
	
	
	/*//curl kerbrose tar to make it
	krbFile :=filepath.Join(a.cacheDir, "archives", "krb.tar.gz")
	krbargs := []string{"-o", krbFile,"-LJO","http://web.mit.edu/kerberos/www/dist/krb5/1.16/krb5-1.16.1.tar.gz"}
	
	if output, err := a.command.Output("/", "curl", krbargs...); err != nil {
	return output, err
	} else {
        fmt.Println("downloaded krb5 tar file in ",krbFile,output)
    	}
	
	//Tar xf the git folder
	tarFolder :=filepath.Join(a.cacheDir, "archives")
	tarargs := []string{"-xf","krb.tar.gz"}
	if output, err := a.command.Output(tarFolder+"/", "tar", tarargs...); err != nil {
	return output, err
	} else {
        fmt.Println("tared of krb5 in ",tarFolder)
    	}
	
	// configure krb5
	sourceFolder := filepath.Join(a.cacheDir, "archives","krb5-1.16.1","src")
	instlocation := "--prefix="+filepath.Join(a.installDir,"usr")
	LDFLAG := "LDFLAG=-L"+filepath.Join(a.installDir,"usr","lib")
	CFLAGS := "CFLAGS=-I"+filepath.Join(a.installDir,"usr","include")
	CPPFLAGS := "CPPFLAGS=-I"+filepath.Join(a.installDir,"usr","include")
	CXXFLAGS := "CXXFLAGS=-I"+filepath.Join(a.installDir,"usr","include")
	configargs := []string{instlocation,LDFLAG,CFLAGS,CPPFLAGS,CXXFLAGS}
	if output, err := a.command.Output(sourceFolder+"/", "./configure", configargs...); err != nil {
	return output, err
	} else {
        fmt.Println("configured of krb5 in ",a.installDir)
    	}
	
	
	// make krb5
	makeargs := []string{}
	if output, err := a.command.Output(sourceFolder+"/", "make", makeargs...); err != nil {
	return output, err
	} else {
        fmt.Println("maked of krb5 in ",tarFolder)
    	}
	
	// make install krb5
	makeinstallargs := []string{"install"}
	if output, err := a.command.Output(sourceFolder+"/", "make", makeinstallargs...); err != nil {
	return output, err
	} else {
        fmt.Println("done make install of krb5 in ",tarFolder)
    	}
	
	
	//curl cyrus sasl tar to make it
	cyrusFile :=filepath.Join(a.cacheDir, "archives", "cyrussasl.tar.gz")
	cyrusargs := []string{"-o", cyrusFile,"-LJO","ftp://ftp.cyrusimap.org/cyrus-sasl/cyrus-sasl-2.1.26.tar.gz"}
	
	if output, err := a.command.Output("/", "curl", cyrusargs...); err != nil {
	return output, err
	} else {
        fmt.Println("downloaded cyrus sasl tar file in ",krbFile,output)
    	}
	
	
	//Tar xf the cyrus sasl folder
	cyrustarFolder :=filepath.Join(a.cacheDir, "archives")
	cyrustarargs := []string{"-xf","cyrussasl.tar.gz"}
	if output, err := a.command.Output(cyrustarFolder+"/", "tar", cyrustarargs...); err != nil {
	return output, err
	} else {
        fmt.Println("tared of cyrus sasl in ",cyrustarFolder)
    	}
	
	
	// configure cyrus sasl
	cyrussourceFolder := filepath.Join(a.cacheDir, "archives","cyrus-sasl-2.1.26")
	cyrusinstlocation := "--prefix="+filepath.Join(a.installDir,"cyrussasl")
	cyrusLDFLAG := "LDFLAG=-L"+filepath.Join(a.installDir,"usr","lib")
	cyrusCFLAGS := "CFLAGS=-I"+filepath.Join(a.installDir,"usr","include")
	cyrusCPPFLAGS := "CPPFLAGS=-I"+filepath.Join(a.installDir,"usr","include")
	cyrusCXXFLAGS := "CXXFLAGS=-I"+filepath.Join(a.installDir,"usr","include")
	cyrusconfigargs := []string{cyrusinstlocation,cyrusLDFLAG,cyrusCFLAGS,cyrusCPPFLAGS,cyrusCXXFLAGS,"--disable-cram","--disable-digest","--disable-otp","--disable-krb4","--disable-plain","--disable-anon"}
	if output, err := a.command.Output(cyrussourceFolder+"/", "./configure", cyrusconfigargs...); err != nil {
	return output, err
	} else {
        fmt.Println("configured of cyrus sasl in ",a.installDir)
    	}
	
	// make cyrus sasl
	cyrusmakeargs := []string{}
	if output, err := a.command.Output(cyrussourceFolder+"/", "make", cyrusmakeargs...); err != nil {
	return output, err
	} else {
        fmt.Println("maked of cyrus sasl in ",cyrustarargs)
    	}
	
	// make install cyrus sasl
	cyrusmakeinstallargs := []string{"install"}
	if output, err := a.command.Output(cyrussourceFolder+"/", "make", cyrusmakeinstallargs...); err != nil {
	return output, err
	} else {
        fmt.Println("done make install of cyrus sasl in ",cyrustarFolder)
    	}*/
	
	
	walkerr := filepath.Walk(a.installDir, visit)
  	fmt.Printf("filepath.Walk() returned %v\n", walkerr)
	
	return "", nil
}
