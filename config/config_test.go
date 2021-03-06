package config

import (
	"io/ioutil"
	"os"
	"path"
	"regexp"
	"strings"
	"testing"

	"github.com/flimzy/diff"
	"github.com/flimzy/testy"
	"github.com/go-kivik/couchdb/chttp"
	"github.com/go-kivik/kouch"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

var expectedConf = &kouch.Config{DefaultContext: "foo",
	Contexts: []kouch.NamedContext{
		{
			Name:    "foo",
			Context: &kouch.Context{Root: "http://foo.com/"},
		},
	},
}

func TestReadConfigFile(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		expected     *kouch.Config
		expectedFile string
		err          string
	}{
		{
			name: "not found",
			err:  "^open /tmp/TestReadConfigFile_not_found-\\d+/config: no such file or directory$",
		},
		{
			name: "yaml input",
			input: `default-context: foo
contexts:
- context:
    root: http://foo.com/
  name: foo
`,
			expected:     expectedConf,
			expectedFile: "^/tmp/TestReadConfigFile_yaml_input-\\d+/config$",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tmpDir := new(string)
			defer testy.TempDir(t, tmpDir)()
			file := path.Join(*tmpDir, "config")
			if test.input != "" {
				if err := ioutil.WriteFile(file, []byte(test.input), 0777); err != nil {
					t.Fatal(err)
				}
			}
			conf, err := readConfigFile(file)
			testy.ErrorRE(t, test.err, err)
			if test.expectedFile != "" {
				if !regexp.MustCompile(test.expectedFile).MatchString(conf.File) {
					t.Errorf("Conf file\nExpected: %s\n  Actual: %s\n", test.expectedFile, conf.File)
				}
				conf.File = ""
			}
			if d := diff.Interface(test.expected, conf); d != nil {
				t.Fatal(d)
			}
		})
	}
}

type rcTest struct {
	name         string
	files        map[string]string
	env          map[string]string
	args         []string
	expected     *kouch.Config
	expectedFile string
	err          string
}

func TestReadConfig(t *testing.T) {
	tests := []rcTest{
		{
			name:     "no config",
			expected: &kouch.Config{},
		},
		{
			name: "default config only",
			files: map[string]string{
				".kouch/config": `default-context: foo
contexts:
- context:
    root: http://foo.com/
  name: foo
`,
			},
			expected:     expectedConf,
			expectedFile: "^/tmp/TestReadConfig_default_config_only-\\d+/.kouch/config$",
		},
		{
			name: "specific config file",
			files: map[string]string{
				"kouch.yaml": `default-context: foo
contexts:
- context:
    root: http://foo.com/
  name: foo
`,
				".kouch/config": `default-context: bar
contexts:
- context:
    root: http://bar.com/
  name: bar
`,
			},
			args:         []string{"--kouchconfig", "${HOME}/kouch.yaml"},
			expected:     expectedConf,
			expectedFile: "^/tmp/TestReadConfig_specific_config_file-\\d+/kouch.yaml$",
		},
		{
			name: "no config, url on command line",
			args: []string{"--root", "foo.com"},
			expected: &kouch.Config{
				DefaultContext: dynamicContextName,
				Contexts: []kouch.NamedContext{
					{
						Name: dynamicContextName,
						Context: &kouch.Context{
							Root: "foo.com",
						},
					},
				},
			},
		},
		{
			name: "default config + username on commandline",
			files: map[string]string{
				".kouch/config": `default-context: foo
contexts:
- context:
    root: http://foo.com/
  name: foo
`,
			},
			args: []string{"--user", "foo"},
			expected: &kouch.Config{DefaultContext: dynamicContextName,
				Contexts: []kouch.NamedContext{
					{
						Name:    "foo",
						Context: &kouch.Context{Root: "http://foo.com/"},
					},
					{
						Name:    dynamicContextName,
						Context: &kouch.Context{User: "foo"},
					},
				},
			},
			expectedFile: "^/tmp/TestReadConfig_default_config_\\+_username_on_commandline-\\d+/.kouch/config$",
		},
		{
			name: "default config + auth on commandline",
			files: map[string]string{
				".kouch/config": `default-context: foo
contexts:
- context:
    root: http://foo.com/
  name: foo
`,
			},
			args: []string{"--user", "foo", "--password", "bar"},
			expected: &kouch.Config{DefaultContext: dynamicContextName,
				Contexts: []kouch.NamedContext{
					{
						Name:    "foo",
						Context: &kouch.Context{Root: "http://foo.com/"},
					},
					{
						Name:    dynamicContextName,
						Context: &kouch.Context{User: "foo", Password: "bar"},
					},
				},
			},
			expectedFile: "^/tmp/TestReadConfig_default_config_\\+_auth_on_commandline-\\d+/.kouch/config$",
		},
		{
			name: "no config, curl-style user/pass combined",
			args: []string{"--root", "foo.com", "--user", "foo:bar"},
			expected: &kouch.Config{
				DefaultContext: dynamicContextName,
				Contexts: []kouch.NamedContext{
					{
						Name: dynamicContextName,
						Context: &kouch.Context{
							Root:     "foo.com",
							User:     "foo",
							Password: "bar",
						},
					},
				},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			readConfigTest(t, test)
		})
	}
}

func readConfigTestCreateTempFiles(t *testing.T, tmpDir *string, files map[string]string) {
	for filename, content := range files {
		file := path.Join(*tmpDir, filename)
		if err := os.MkdirAll(path.Dir(file), 0777); err != nil {
			t.Fatal(err)
		}
		if err := ioutil.WriteFile(file, []byte(content), 0777); err != nil {
			t.Fatal(err)
		}
	}
}

func readConfigTest(t *testing.T, test rcTest) {
	tmpDir := new(string)
	defer testy.TempDir(t, tmpDir)()
	defer testy.RestoreEnv()()
	env := map[string]string{"HOME": *tmpDir}
	for k, v := range test.env {
		env[k] = strings.Replace(v, "${HOME}", *tmpDir, -1)
	}
	if e := testy.SetEnv(env); e != nil {
		t.Fatal(e)
	}
	readConfigTestCreateTempFiles(t, tmpDir, test.files)

	cmd := &cobra.Command{}
	AddFlags(cmd.PersistentFlags())
	for i, v := range test.args {
		test.args[i] = strings.Replace(v, "${HOME}", *tmpDir, -1)
	}
	if e := cmd.ParseFlags(test.args); e != nil {
		t.Fatal(e)
	}

	conf, err := ReadConfig(cmd)
	testy.ErrorRE(t, test.err, err)
	if test.expectedFile != "" {
		if !regexp.MustCompile(test.expectedFile).MatchString(conf.File) {
			t.Errorf("Conf file\nExpected: %s\n  Actual: %s\n", test.expectedFile, conf.File)
		}
		conf.File = ""
	}
	if d := diff.Interface(test.expected, conf); d != nil {
		t.Fatal(d)
	}
}

func TestConstructContext(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		expected *kouch.Context
		err      string
		status   int
	}{
		{
			name:     "standard root url",
			args:     []string{"--root", "http://foo.com/"},
			expected: &kouch.Context{Root: "http://foo.com/"},
		},
		{
			name: "auth credentials",
			args: []string{"--root", "http://foo.com/", "--user", "foo", "--password", "bar"},
			expected: &kouch.Context{
				Root:     "http://foo.com/",
				User:     "foo",
				Password: "bar",
			},
		},
		{
			name: "curl-style combined creds",
			args: []string{"--root", "http://foo.com/", "--user", "foo:bar"},
			expected: &kouch.Context{
				Root:     "http://foo.com/",
				User:     "foo",
				Password: "bar",
			},
		},
		{
			name: "url with credentials",
			args: []string{"--root", "http://foo:bar@foo.com/"},
			expected: &kouch.Context{
				Root:     "http://foo.com/",
				User:     "foo",
				Password: "bar",
			},
		},
		{
			name:   "invalid url",
			args:   []string{"--root", "http://foo.com/%xx"},
			err:    `parse http://foo.com/%xx: invalid URL escape "%xx"`,
			status: chttp.ExitStatusURLMalformed,
		},
		{
			name:     "nothing",
			expected: nil,
		},
		{
			name: "url with credentials, and command line user",
			args: []string{"--root", "http://abc:xyz@foo.com/", "--user", "foo"},
			expected: &kouch.Context{
				Root: "http://foo.com/",
				User: "foo",
			},
		},
		{
			name: "url with credentials, and command line password",
			args: []string{"--root", "http://abc:xyz@foo.com/", "--password", "foo"},
			expected: &kouch.Context{
				Root:     "http://foo.com/",
				User:     "abc",
				Password: "foo",
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cmd := &cobra.Command{}
			AddFlags(cmd.PersistentFlags())
			if e := cmd.ParseFlags(test.args); e != nil {
				t.Fatal(e)
			}

			result, err := constructContext(cmd.Flags())
			testy.ExitStatusError(t, test.err, test.status, err)
			if d := diff.Interface(test.expected, result); d != nil {
				t.Error(d)
			}
		})
	}
}

func TestCredentials(t *testing.T) {
	tests := []struct {
		name         string
		user, pass   string
		flags        *pflag.FlagSet
		eUser, ePass string
		err          string
	}{
		{
			name:  "No auth flags",
			user:  "foo",
			pass:  "bar",
			flags: pflag.NewFlagSet("foo", 1),
			eUser: "foo",
			ePass: "bar",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := credentials(&test.user, &test.pass, test.flags)
			testy.Error(t, test.err, err)
			if test.user != test.eUser || test.pass != test.ePass {
				t.Errorf("Unexpected results.\n Got: %s/%s\nWant: %s/%s\n",
					test.user, test.pass, test.eUser, test.ePass)
			}
		})
	}
}
