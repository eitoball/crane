package crane

import (
	"github.com/stretchr/testify/assert"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"syscall"
	"testing"
)

// Create a map of stubbed containers out of the provided set
func NewStubbedContainerMap(exists bool, containers ...Container) ContainerMap {
	containerMap := make(map[string]Container)
	for _, container := range containers {
		containerMap[container.Name()] = &StubbedContainer{container, exists}
	}
	return containerMap
}

type StubbedContainer struct {
	Container
	exists bool
}

func (stubbedContainer *StubbedContainer) Exists() bool {
	return stubbedContainer.exists
}

func TestConfigFilenames(t *testing.T) {
	// With given fileName
	fileName := "some/file.yml"
	files := configFilenames(fileName)
	assert.Equal(t, []string{fileName}, files)
	// Without given fileName
	files = configFilenames("")
	assert.Equal(t, []string{"crane.json", "crane.yaml", "crane.yml"}, files)
}

func TestFindConfig(t *testing.T) {
	f, _ := ioutil.TempFile("", "crane.yml")
	defer syscall.Unlink(f.Name())
	configName := filepath.Base(f.Name())
	absConfigName := os.TempDir() + "/" + configName
	var fileName string

	// Finds config in current dir
	os.Chdir(os.TempDir())
	fileName = findConfig(configName)
	assert.Equal(t, f.Name(), fileName)

	// Finds config in parent dir
	d, _ := ioutil.TempDir("", "sub")
	defer syscall.Unlink(d)
	os.Chdir(d)
	fileName = findConfig(configName)
	assert.Equal(t, f.Name(), fileName)

	// Finds config with absolute path
	fileName = findConfig(absConfigName)
	assert.Equal(t, f.Name(), fileName)
}

func TestUnmarshal(t *testing.T) {
	var actual *config
	json := []byte(
		`{
    "containers": {
        "apache": {
            "dockerfile": "apache",
            "image": "michaelsauter/apache",
            "run": {
                "volumes-from": ["crane_app"],
                "publish": ["80:80"],
                "link": ["crane_mysql:db", "crane_memcached:cache"],
                "detach": true
            }
        }
    },
    "groups": {
        "default": [
            "apache"
        ]
    },
    "hooks": {
        "apache": {
            "post-stop": "echo apache container stopped!\n"
        },
        "default": {
            "pre-start": "echo start...",
            "post-start": "echo start done!\n"
        }
    }
}
`)
	actual = unmarshal(json, ".json")
	assert.Len(t, actual.RawContainerMap, 1)
	assert.Len(t, actual.RawContainerMap["apache"].RunParams.Link(), 2)
	assert.Len(t, actual.RawGroups, 1)
	assert.Len(t, actual.RawHooksMap, 2)
	assert.NotEmpty(t, actual.RawHooksMap["default"].RawPreStart)
	assert.NotEmpty(t, actual.RawHooksMap["default"].RawPostStart)

	yaml := []byte(
		`containers:
  apache:
    dockerfile: apache
    image: michaelsauter/apache
    run:
      volumes-from: ["crane_app"]
      publish: ["80:80"]
      link: ["crane_mysql:db", "crane_memcached:cache"]
      detach: true
groups:
  default:
    - apache
hooks:
  apache:
    post-stop: echo apache container stopped!\n
  default:
    pre-start: echo start...
    post-start: echo start done!\n
`)
	actual = unmarshal(yaml, ".yml")
	assert.Len(t, actual.RawContainerMap, 1)
	assert.Len(t, actual.RawContainerMap["apache"].RunParams.Link(), 2)
	assert.Len(t, actual.RawGroups, 1)
	assert.Len(t, actual.RawHooksMap, 2)
	assert.NotEmpty(t, actual.RawHooksMap["default"].RawPreStart)
	assert.NotEmpty(t, actual.RawHooksMap["default"].RawPostStart)
}

func TestUnmarshalInvalidJSON(t *testing.T) {
	json := []byte(
		`{
    "containers": {
        "apache": {
            "image": "michaelsauter/apache",
            "run": {
                "publish": "shouldbeanarray"
            }
        }
    }
}
`)
	assert.Panics(t, func() {
		unmarshal(json, ".json")
	})
}

func TestUnmarshalInvalidYAML(t *testing.T) {
	yaml := []byte(
		`containers:
  apache:
    image: michaelsauter/apache
    run:
      publish: "shouldbeanarray"
`)
	assert.Panics(t, func() {
		unmarshal(yaml, ".yml")
	})
}

func TestInitialize(t *testing.T) {
	// use different, undefined environment variables throughout the config to detect any issue in expansion
	rawContainerMap := map[string]*container{
		"${UNDEFINED1}a": &container{},
		"${UNDEFINED2}b": &container{},
	}
	rawGroups := map[string][]string{
		"${UNDEFINED3}default": []string{
			"${UNDEFINED4}a",
			"${UNDEFINED4}b",
		},
	}
	rawHooksMap := map[string]hooks{
		"${UNDEFINED5}default": hooks{
			RawPreStart:  "${UNDEFINED6}default-pre-start",
			RawPostStart: "${UNDEFINED7}default-post-start",
		},
		"${UNDEFINED8}a": hooks{
			RawPreStart: "${UNDEFINED9}custom-pre-start",
		},
	}
	c := &config{
		RawContainerMap: rawContainerMap,
		RawGroups:       rawGroups,
		RawHooksMap:     rawHooksMap,
	}
	c.initialize()
	assert.Equal(t, "a", c.containerMap["a"].Name())
	assert.Equal(t, "b", c.containerMap["b"].Name())
	assert.Equal(t, map[string][]string{"default": []string{"a", "b"}}, c.groups)
	assert.Equal(t, "custom-pre-start", c.containerMap["a"].Hooks().PreStart(), "Container should have a custom pre-start hook overriding the default one")
	assert.Equal(t, "default-post-start", c.containerMap["a"].Hooks().PostStart(), "Container should have a default post-start hook")
	assert.Equal(t, "default-pre-start", c.containerMap["b"].Hooks().PreStart(), "Container should have a default post-start hook")
	assert.Equal(t, "default-post-start", c.containerMap["b"].Hooks().PostStart(), "Container should have a default post-start hook")
}

func TestInitializeAmbiguousHooks(t *testing.T) {
	rawContainerMap := map[string]*container{
		"a": &container{},
		"b": &container{},
	}
	rawGroups := map[string][]string{
		"group1": []string{"a"},
		"group2": []string{"a", "b"},
	}
	rawHooksMap := map[string]hooks{
		"group1": hooks{RawPreStart: "group1-pre-start"},
		"group2": hooks{RawPreStart: "group2-pre-start"},
	}
	c := &config{
		RawContainerMap: rawContainerMap,
		RawGroups:       rawGroups,
		RawHooksMap:     rawHooksMap,
	}
	assert.Panics(t, func() {
		c.initialize()
	})
}

func TestOverrideImageTag(t *testing.T) {
	rawContainers := []*container {
		&container{RawName: "full-spec",	RawImage: "test/image-a:1.0"},
		&container{RawName: "without-repo",	RawImage: "image-b:latest"},
		&container{RawName: "without-tag",	RawImage: "test/image-c"},
		&container{RawName: "image-only",	RawImage: "image-d"},
		&container{RawName: "private-registry",	RawImage: "localhost:5000/foo/image-e:2.0"},
		&container{RawName: "digest",		RawImage: "localhost:5000/foo/image-f@sha256:xxx"},
	}
	rawContainerMap := make(map[string]*container)
	for _, container := range rawContainers {
		rawContainerMap[container.Name()] = container
	}
	cfg = &config {	// container.prefiexedName() depends cfg object...
	}
	c := &config {
		RawContainerMap: rawContainerMap,
		tag: "rc-1",
	}

	os.Setenv("CRANE_TAG", "default-tag")

	c.initialize()
	c.overrideImageTag()

	assert.Equal(t, "test/image-a:rc-1", c.containerMap["full-spec"].Image())
	assert.Equal(t, "full-spec", c.containerMap["full-spec"].ActualName())

	assert.Equal(t, "image-b:rc-1", c.containerMap["without-repo"].Image())
	assert.Equal(t, "without-repo", c.containerMap["without-repo"].ActualName())

	assert.Equal(t, "test/image-c:rc-1", c.containerMap["without-tag"].Image())
	assert.Equal(t, "without-tag", c.containerMap["without-tag"].ActualName())

	assert.Equal(t, "image-d:rc-1", c.containerMap["image-only"].Image())
	assert.Equal(t, "image-only", c.containerMap["image-only"].ActualName())

	assert.Equal(t, "localhost:5000/foo/image-e:rc-1", c.containerMap["private-registry"].Image())
	assert.Equal(t, "private-registry", c.containerMap["private-registry"].ActualName())

	assert.NotEqual(t, "localhost:5000/foo/image-f@sha256:rc-1", c.containerMap["digest"].Image())
	assert.Equal(t, "digest", c.containerMap["digest"].ActualName())

	assert.Equal(t, "rc-1", os.Getenv("CRANE_TAG"))
}

func TestDependencyGraph(t *testing.T) {
	containerMap := NewStubbedContainerMap(true,
		&container{RawName: "a", RunParams: RunParameters{RawLink: []string{"b:b"}}},
		&container{RawName: "b", RunParams: RunParameters{RawLink: []string{"c:c"}}},
		&container{RawName: "c"},
	)
	c := &config{containerMap: containerMap}

	dependencyGraph := c.DependencyGraph([]string{})
	assert.Len(t, dependencyGraph, 3)
	// make sure a new graph is returned each time
	dependencyGraph.resolve("a") // mutate the previous graph
	assert.Len(t, c.DependencyGraph([]string{}), 3)

	dependencyGraph = c.DependencyGraph([]string{"b"})
	assert.Len(t, dependencyGraph, 2)
}

func TestContainersForReference(t *testing.T) {
	var containers []string
	containerMap := NewStubbedContainerMap(true,
		&container{RawName: "a"},
		&container{RawName: "b"},
		&container{RawName: "c"},
	)

	// No target given
	// If default group exist, it returns its containers
	groups := map[string][]string{
		"default": []string{"a", "b"},
	}
	c := &config{containerMap: containerMap, groups: groups}
	containers = c.ContainersForReference("")
	assert.Equal(t, []string{"a", "b"}, containers)
	// If no default group, returns all containers
	c = &config{containerMap: containerMap}
	containers = c.ContainersForReference("")
	sort.Strings(containers)
	assert.Equal(t, []string{"a", "b", "c"}, containers)
	// Target given
	// Target is a group
	groups = map[string][]string{
		"second": []string{"b", "c"},
	}
	c = &config{containerMap: containerMap, groups: groups}
	containers = c.ContainersForReference("second")
	assert.Equal(t, []string{"b", "c"}, containers)
	// Target is a container
	containers = c.ContainersForReference("a")
	assert.Equal(t, []string{"a"}, containers)
}

func TestContainersForReferenceInvalidReference(t *testing.T) {
	containerMap := NewStubbedContainerMap(true,
		&container{RawName: "a"},
		&container{RawName: "b"},
	)
	groups := map[string][]string{
		"foo": []string{"a", "doesntexist", "b"},
	}
	c := &config{containerMap: containerMap, groups: groups}
	assert.Panics(t, func() {
		c.ContainersForReference("foo")
	})
	assert.Panics(t, func() {
		c.ContainersForReference("doesntexist")
	})
}

func TestContainersForReferenceDeduplication(t *testing.T) {
	containerMap := NewStubbedContainerMap(true,
		&container{RawName: "a"},
		&container{RawName: "b"},
	)
	groups := map[string][]string{
		"foo": []string{"a", "b", "a"},
	}
	c := &config{containerMap: containerMap, groups: groups}
	containers := c.ContainersForReference("foo")
	assert.Equal(t, []string{"a", "b"}, containers)
}
