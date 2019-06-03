package cmd

import (
	"encoding/json"
	"fmt"
	kudov1alpha1 "github.com/kudobuilder/kudo/pkg/apis/kudo/v1alpha1"
	"github.com/spf13/cobra"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/remotecommand"
	"log"
	"net/http"
	"os"
	"os/user"
	"path/filepath"
	"strings"
)

const (
	defaultConfigPath = ".kube/config"
)

var (
	kubeConfig string
	namespace  string
	name       string
)

// NewOperatorCmd used for custom operator commands
func NewOperatorCmd() *cobra.Command {
	operatorCmd := &cobra.Command{
		Use:   "operator",
		Short: "-> Use operator commands.",
		Long:  `The operator command has subcommands defined in framework definitions`,
		Run:   operatorCmdExample,
	}

	operatorCmd.Flags().StringVar(&kubeConfig, "kubeconfig", "", "The file path to kubernetes configuration file; defaults to $HOME/.kube/config")
	operatorCmd.Flags().StringVar(&namespace, "namespace", "default", "The namespace where the operator watches for changes.")
	operatorCmd.Flags().StringVar(&namespace, "pod", "", "The framework for which we want to execute the command")


	return operatorCmd
}

func operatorCmdExample(cmd *cobra.Command, args []string) {
	mustKubeConfig()
	_, err := cmd.Flags().GetString("kubeconfig")
	if err != nil {
		log.Printf("Flag Error: %v", err)
	}

	if len(args) > 0 {
		pods, _ := getFrameworkPods(args[0])
		if len(pods.Items) > 0 {
			pod := pods.Items[0]
			frameworks, _ := getFrameworkVersions(args[0])
			if frameworks != nil && len(frameworks.Items) > 0 {
				testArray := strings.Fields(frameworks.Items[0].Spec.Commands[0].RunCommand)
				ExecInPod(&pod, pods.Items[0].Spec.Containers[0].Name, testArray)
			}
		}

	}

}

func getFrameworkPods(name string) (*v1.PodList, error) {
	config, err := clientcmd.BuildConfigFromFlags("", kubeConfig)
	if err != nil {
		return nil, err
	}

	//  Create a Dynamic Client to interface with CRDs.
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	listOptions := metav1.ListOptions{LabelSelector: fmt.Sprintf("%s=%s", name, name)}
	if pods, err := clientset.CoreV1().Pods(namespace).List(listOptions); err == nil {
		return pods, nil
	}
	return nil, fmt.Errorf("no pods found for %s", name)

}

func getFrameworkVersions(name string) (*kudov1alpha1.FrameworkVersionList, error) {
	config, err := clientcmd.BuildConfigFromFlags("", kubeConfig)
	if err != nil {
		return nil, err
	}

	//  Create a Dynamic Client to interface with CRDs.
	dynamicClient, err := dynamic.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	frameworksGVR := schema.GroupVersionResource{
		Group:    "kudo.k8s.io",
		Version:  "v1alpha1",
		Resource: "frameworkversions",
	}
	listOptions := metav1.ListOptions{LabelSelector: fmt.Sprintf("app=%s", name)}

	instObj, err := dynamicClient.Resource(frameworksGVR).Namespace(namespace).List(listOptions)
	if err != nil {
		log.Printf("Error: %v", err)
		return nil, err
	}

	mInstObj, _ := instObj.MarshalJSON()

	instance := kudov1alpha1.FrameworkVersionList{}

	err = json.Unmarshal(mInstObj, &instance)
	if err != nil {
		return nil, err
	}

	return &instance, nil
}

// mustKubeConfig checks if the kubeconfig file exists.
func mustKubeConfig() {
	// if kubeConfig is not specified, search for the default kubeconfig file under the $HOME/.kube/config.
	if len(kubeConfig) == 0 {
		usr, err := user.Current()
		if err != nil {
			fmt.Printf("Error: failed to determine user's home dir: %v", err)
		}
		kubeConfig = filepath.Join(usr.HomeDir, defaultConfigPath)
	}

	_, err := os.Stat(kubeConfig)
	if err != nil && os.IsNotExist(err) {
		fmt.Printf("Error: failed to find the kubeconfig file (%v): %v", kubeConfig, err)
	}
}

func ExecInPod(pod *v1.Pod, container string, commands []string) (string, error) {

	config, err := clientcmd.BuildConfigFromFlags("", kubeConfig)
	if err != nil {
		return "", err
	}
	//  Create a Dynamic Client to interface with CRDs.
	clientset, err := kubernetes.NewForConfig(config)
	restClient := clientset.CoreV1().RESTClient()

	req := restClient.Post().
		Namespace(pod.Namespace).
		Resource("pods").
		Name(pod.Name).
		SubResource("exec").
		Param("container", container).
		Param("stdout", "true").
		Param("stderr", "true")

	for _, command := range commands {
		req.Param("command", command)
	}

	executor, err := remotecommand.NewSPDYExecutor(config, http.MethodPost, req.URL())
	if err != nil {
		return "", err
	}



	err = executor.Stream(remotecommand.StreamOptions{
		Stdin:            nil,
		Stdout:            os.Stdout,
		Stderr:            os.Stderr,
		Tty:               true,
		TerminalSizeQueue: nil,
	})


	return "", err
}
