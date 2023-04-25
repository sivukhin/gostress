package main

import (
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

const (
	GoStressEnv    = "GOSTRESSENV"
	GoStressEnvK8s = "K8S"
)

var (
	shutdownTimeout = 1 * time.Minute
	waitTimeout     = 1 * time.Minute
	waitInterval    = 200 * time.Millisecond
)

func K8sNormalize(s string) string {
	s = strings.ReplaceAll(s, "/", "-")
	s = strings.ReplaceAll(s, ".", "-")
	s = strings.ReplaceAll(s, "_", "-")
	return strings.ToLower(s)
}

func (s *Stress) RunK8s(ctx context.Context, namespace string, modifiers ...PodOpts) {
	if os.Getenv(GoStressEnv) == GoStressEnvK8s {
		s.Logger.Infof("detected k8s environment, run test locally")
		s.RunLocal(ctx)
		return
	}
	s.runK8s(ctx, namespace, modifiers...)
}

func (s *Stress) runK8s(ctx context.Context, namespace string, modifiers ...PodOpts) {
	kube, err := NewKubeContext(s.Logger)
	if err != nil {
		panic(fmt.Errorf("unable to create k8s context: %w", err))
	}
	podObject := kube.GetPodObject(namespace, fmt.Sprintf("gostress-%v-%v", s.Name, s.Nonce), modifiers...)
	s.Logger.Infof("ready to create one time pod in namespace %v", namespace)
	kubePod, err := kube.CreateOneTimePod(ctx, podObject)
	if err != nil {
		panic(fmt.Errorf("unable to create k8s pod: %v", err))
	}
	defer kubePod.Shutdown(shutdownTimeout)
	s.Logger.Infof("created pod in namespace %v: %v", namespace, kubePod.object.Name)

	err = kubePod.Wait(waitTimeout, waitInterval)
	if err != nil {
		panic(fmt.Errorf("unable to wait for k8s pod to initialize: %v", kubePod.object.Name))
	}
	s.Logger.Infof("pod %v is ready", kubePod.object.Name)

	podOs, podArch, err := kubePod.Workspace().Info(ctx)
	if err != nil {
		panic(fmt.Errorf("unable to get os/arch in pod: %v", err))
	}
	s.Logger.Infof("determined os/arch on pod %v: os=%v, arch=%v", kubePod.object.Name, podOs, podArch)

	workspace := kubePod.Workspace()
	testName := workspace.Name()
	err = BuildCurrentTest(testName, podOs, podArch)
	if err != nil {
		panic(err)
	}
	root, cwd, err := GetCurrentModuleRoot()
	if err != nil {
		panic(err)
	}
	s.Logger.Infof("found go module root at %v", root)
	err = workspace.CopyDir(ctx, root)
	if err != nil {
		panic(err)
	}
	rel, err := filepath.Rel(root, cwd)
	if err != nil {
		panic(fmt.Errorf("unable to get relative path for %v against %v: %w", cwd, root, err))
	}
	_, err = workspace.Exec(ctx, fmt.Sprintf("%v -test.run %v -test.v", path.Join(workspace.Directory(), rel, workspace.Name()), s.Name))
	if err != nil {
		panic(err)
	}
}
