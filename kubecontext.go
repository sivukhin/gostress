package main

import (
	"archive/tar"
	"bufio"
	"bytes"
	"context"
	"fmt"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"io"
	"io/fs"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	v1meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/remotecommand"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type KubeContext struct {
	logger *zap.SugaredLogger
	config *rest.Config
	client *kubernetes.Clientset
}

type KubePod struct {
	kube   *KubeContext
	object *v1.Pod
}

type KubePodWorkspace struct {
	pod  *KubePod
	name string
}

func NewKubeContext(logger *zap.SugaredLogger) (*KubeContext, error) {
	kubeConfig := filepath.Join(os.Getenv("HOME"), ".kube", "config")
	config, err := clientcmd.BuildConfigFromFlags("", kubeConfig)
	if err != nil {
		return nil, fmt.Errorf("unable to create k8s config: %w", err)
	}
	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("unable to create k8s client: %w", err)
	}
	return &KubeContext{logger: logger, client: client, config: config}, nil
}

type podOpts struct {
	name        string
	metricsPort int
	goVersion   string
	cpu         string
	memory      string
	labels      map[string]string
	timeout     time.Duration
}

type PodOpts interface{ apply(opts *podOpts) }

type (
	podMetricsPort int
	podGoVersion   string
	podCpu         string
	podMemory      string
	podLabels      map[string]string
	podAutoLabels  struct{}
	podTimeout     time.Duration
)

func (p podMetricsPort) apply(opts *podOpts) { opts.metricsPort = int(p) }
func (p podGoVersion) apply(opts *podOpts)   { opts.goVersion = string(p) }
func (p podCpu) apply(opts *podOpts)         { opts.cpu = string(p) }
func (p podMemory) apply(opts *podOpts)      { opts.memory = string(p) }
func (p podTimeout) apply(opts *podOpts)     { opts.timeout = time.Duration(p) }
func (p podAutoLabels) apply(opts *podOpts)  { opts.labels["id"] = opts.name }
func (p podLabels) apply(opts *podOpts) {
	for k, v := range p {
		opts.labels[k] = v
	}
}

func WithMetricsPort(port int) PodOpts               { return podMetricsPort(port) }
func WithGoVersion(goVersion string) PodOpts         { return podGoVersion(goVersion) }
func WithCpu(cpu string) PodOpts                     { return podCpu(cpu) }
func WithMemory(memory string) PodOpts               { return podMemory(memory) }
func WithTimeout(timeout time.Duration) PodOpts      { return podTimeout(timeout) }
func WithPodAutoLabels() PodOpts                     { return podAutoLabels{} }
func WithPodLabels(labels map[string]string) PodOpts { return podLabels(labels) }

func (kube *KubeContext) GetPodObject(namespace, name string, modifiers ...PodOpts) *v1.Pod {
	options := podOpts{
		name:        name,
		metricsPort: 3000,
		goVersion:   "1.20",
		cpu:         "1",
		memory:      "1Gi",
		timeout:     5 * time.Hour,
		labels:      make(map[string]string, 0),
	}
	for _, modifier := range modifiers {
		modifier.apply(&options)
	}
	return &v1.Pod{
		ObjectMeta: v1meta.ObjectMeta{
			Namespace: namespace,
			Name:      K8sNormalize(options.name),
			Labels:    options.labels,
			Annotations: map[string]string{
				"prometheus.io/scrape": "true",
				"prometheus.io/port":   strconv.Itoa(options.metricsPort),
			},
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				{
					Name:  "go",
					Image: "golang:1.20-bullseye",
					Resources: v1.ResourceRequirements{
						Limits: v1.ResourceList{
							v1.ResourceCPU:    resource.MustParse(options.cpu),
							v1.ResourceMemory: resource.MustParse(options.memory),
						},
						Requests: v1.ResourceList{
							v1.ResourceCPU:    resource.MustParse(options.cpu),
							v1.ResourceMemory: resource.MustParse(options.memory),
						},
					},
					Command: []string{"sleep", strconv.Itoa(int(options.timeout.Seconds()))},
					Env: []v1.EnvVar{
						{Name: GoStressEnv, Value: GoStressEnvK8s},
					},
				},
			},
			DNSPolicy:     v1.DNSDefault,
			RestartPolicy: v1.RestartPolicyNever,
		},
	}
}

func (kube *KubeContext) CreateOneTimePod(ctx context.Context, podObject *v1.Pod) (*KubePod, error) {
	kube.logger.Infof("creating k8s pod %v", podObject.Name)
	pod, err := kube.client.CoreV1().Pods(podObject.Namespace).Create(ctx, podObject, v1meta.CreateOptions{})
	if err != nil {
		kube.logger.Errorf("failed to create k8s pod %v: %v", podObject.Name, err)
		return nil, fmt.Errorf("failed to create k8s pod %v: %w", podObject.Name, err)
	} else {
		kube.logger.Errorf("succeed to create k8s pod %v", pod.Name)
	}
	return &KubePod{kube: kube, object: pod}, nil
}

func (pod *KubePod) Wait(timeout, interval time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("unable to wait for pod %v", pod.object.Name)
		default:
		}
		current, err := pod.kube.client.CoreV1().Pods(pod.object.Namespace).Get(ctx, pod.object.Name, v1meta.GetOptions{})
		if err != nil {
			pod.kube.logger.Errorf("failed to get information for pod %v: %v", pod.object.Name, err)
		}
		if current.Status.Phase == v1.PodRunning {
			break
		}
		pod.kube.logger.Debugf("found pod %v in phase %v", pod.object.Name, current.Status.Phase)
		time.Sleep(interval)
	}
	return nil
}

func (pod *KubePod) Shutdown(timeout time.Duration) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	pod.kube.logger.Infof("deleting k8s pod %v", pod.object.Name)
	err := pod.kube.client.CoreV1().Pods(pod.object.Namespace).Delete(ctx, pod.object.Name, v1meta.DeleteOptions{})
	if err != nil {
		pod.kube.logger.Errorf("failed to delete k8s pod %v: %v", pod.object.Name, err)
	} else {
		pod.kube.logger.Infof("succeed to delete k8s pod %v", pod.object.Name)
	}
}

func (pod *KubePod) Workspace() *KubePodWorkspace {
	return &KubePodWorkspace{pod: pod, name: fmt.Sprintf("gostress-%v", uuid.Must(uuid.NewUUID()).String()[:8])}
}

func (workspace *KubePodWorkspace) Name() string { return workspace.name }

func (workspace *KubePodWorkspace) Directory() string {
	return fmt.Sprintf("/work/%v", workspace.name)
}

var scheme = runtime.NewScheme()
var codec = runtime.NewParameterCodec(scheme)

func init() {
	scheme.AddKnownTypes(schema.GroupVersion{Group: "meta.k8s.io", Version: "v1"}, &v1.PodExecOptions{})
}

func (workspace *KubePodWorkspace) Exec(ctx context.Context, c string) ([]string, error) {
	command := []string{"bash", "-c", fmt.Sprintf("mkdir -p %v && cd %v && %v", workspace.Directory(), workspace.Directory(), c)}
	req := workspace.pod.kube.client.
		RESTClient().
		Post().
		Prefix("api", "v1").
		Resource("pods").
		Namespace(workspace.pod.object.Namespace).
		Name(workspace.pod.object.Name).
		SubResource("exec").
		VersionedParams(&v1.PodExecOptions{
			Container: workspace.pod.object.Spec.Containers[0].Name,
			Command:   command,
			Stdin:     false,
			Stdout:    true,
			Stderr:    true,
		}, codec)
	workspace.pod.kube.logger.Infof("ready to execute command: %#v", command)
	exec, err := remotecommand.NewSPDYExecutor(workspace.pod.kube.config, "POST", req.URL())
	if err != nil {
		return nil, fmt.Errorf("unable to create SPDY executor: %w", err)
	}
	output := make([]string, 0)
	stdoutReader, stdoutWriter := io.Pipe()
	stderrReader, stderrWriter := io.Pipe()
	go func() {
		reader := bufio.NewScanner(stdoutReader)
		for reader.Scan() {
			output = append(output, reader.Text())
			workspace.pod.kube.logger.Infof("[stdout]: %v", reader.Text())
		}
	}()
	go func() {
		reader := bufio.NewScanner(stderrReader)
		for reader.Scan() {
			workspace.pod.kube.logger.Errorf("[stderr]: %v", reader.Text())
		}
	}()
	err = exec.StreamWithContext(ctx, remotecommand.StreamOptions{
		Stdin:             nil,
		Stdout:            stdoutWriter,
		Stderr:            stderrWriter,
		Tty:               false,
		TerminalSizeQueue: nil,
	})
	if err != nil {
		return nil, fmt.Errorf("unable to execute command %#v: %w", command, err)
	}
	workspace.pod.kube.logger.Infof("succeed with command %#v", command)
	return output, nil
}

func (workspace *KubePodWorkspace) execStdin(ctx context.Context, content []byte, c string) error {
	command := []string{"bash", "-c", fmt.Sprintf("mkdir -p %v && cd %v && %v", workspace.Directory(), workspace.Directory(), c)}
	req := workspace.pod.kube.client.
		RESTClient().
		Post().
		Prefix("api", "v1").
		Resource("pods").
		Namespace(workspace.pod.object.Namespace).
		Name(workspace.pod.object.Name).
		SubResource("exec").
		VersionedParams(&v1.PodExecOptions{
			Container: workspace.pod.object.Spec.Containers[0].Name,
			Command:   command,
			Stdin:     true,
			Stdout:    true,
			Stderr:    true,
		}, codec)
	workspace.pod.kube.logger.Infof("ready to execute command: %#v", command)

	exec, err := remotecommand.NewSPDYExecutor(workspace.pod.kube.config, "POST", req.URL())
	if err != nil {
		return fmt.Errorf("unable to create SPDY executor: %w", err)
	}
	stdoutReader, stdoutWriter := io.Pipe()
	stderrReader, stderrWriter := io.Pipe()
	go func() {
		reader := bufio.NewScanner(stdoutReader)
		for reader.Scan() {
			workspace.pod.kube.logger.Infof("[stdout]: %v", reader.Text())
		}
	}()
	go func() {
		reader := bufio.NewScanner(stderrReader)
		for reader.Scan() {
			workspace.pod.kube.logger.Errorf("[stderr]: %v", reader.Text())
		}
	}()
	err = exec.StreamWithContext(ctx, remotecommand.StreamOptions{
		Stdin:             bytes.NewReader(content),
		Stdout:            stdoutWriter,
		Stderr:            stderrWriter,
		Tty:               false,
		TerminalSizeQueue: nil,
	})
	if err != nil {
		return fmt.Errorf("unable to execute command %#v: %w", command, err)
	}
	workspace.pod.kube.logger.Infof("succeed with command %#v", command)
	return nil
}

func (workspace *KubePodWorkspace) CopyContent(ctx context.Context, content []byte, target string) error {
	buffer := bytes.NewBuffer(make([]byte, 0))
	pack := tar.NewWriter(buffer)
	err := pack.WriteHeader(&tar.Header{
		Name:    target,
		Size:    int64(len(content)),
		Mode:    0600,
		ModTime: time.Now(),
	})
	if err != nil {
		return err
	}
	for len(content) > 0 {
		n, err := pack.Write(content)
		if err != nil {
			return err
		}
		content = content[n:]
	}
	err = pack.Close()
	if err != nil {
		return err
	}
	return workspace.execStdin(ctx, buffer.Bytes(), fmt.Sprintf("tar -xmf - %v", target))
}

func (workspace *KubePodWorkspace) Info(ctx context.Context) (os, arch string, err error) {
	var output []string
	output, err = workspace.Exec(ctx, "go version")
	if err != nil {
		return
	}
	tokens := strings.Split(output[0], " ")
	components := strings.Split(tokens[len(tokens)-1], "/")
	os, arch = components[0], components[1]
	workspace.pod.kube.logger.Infof("os: '%v', arch: '%v'", os, arch)
	return
}

func (workspace *KubePodWorkspace) CopyDir(ctx context.Context, directory string) error {
	buffer := bytes.NewBuffer(make([]byte, 0))
	pack := tar.NewWriter(buffer)
	err := filepath.Walk(directory, func(path string, info fs.FileInfo, err error) error {
		header, err := tar.FileInfoHeader(info, path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(directory, path)
		if err != nil {
			return err
		}
		header.Name = filepath.ToSlash(rel)
		if err := pack.WriteHeader(header); err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		data, err := os.Open(path)
		if err != nil {
			return err
		}
		if _, err := io.Copy(pack, data); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return err
	}
	if err = pack.Close(); err != nil {
		return err
	}
	return workspace.execStdin(ctx, buffer.Bytes(), "tar -xmf -")
}
