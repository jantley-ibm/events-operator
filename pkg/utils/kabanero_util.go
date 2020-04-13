/*
Copyright 2020 IBM Corporation

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package utils

import (
    "context"
	"fmt"
	kabanerov1alpha2 "github.com/kabanero-io/kabanero-operator/pkg/apis/kabanero/v1alpha2"
    "github.com/kabanero-io/events-operator/pkg/semverimage"
	"k8s.io/client-go/rest"
	"net/url"

	//"io/ioutil"
 	corev1 "k8s.io/api/core/v1"
	// metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	// "k8s.io/client-go/kubernetes"
    "sigs.k8s.io/controller-runtime/pkg/client"

    triggers "github.com/tektoncd/triggers/pkg/apis/triggers/v1alpha1"

	// "k8s.io/client-go/rest"
	"k8s.io/klog"
	// "net/url"
	//"os"
	"strings"
)

const (
	// KUBENAMESPACE the namespace that kabanero is running in
//	KUBENAMESPACE = "KUBE_NAMESPACE"
	// DEFAULTNAMESPACE the default namespace name
//	DEFAULTNAMESPACE = "kabanero"

    ACTIVE = "active"
    TEKTON_DEV = "tekton.dev"
    EVENT_LISTENER = "EventListener"
)

/*
var (
	kabaneroNamespace string
)
*/

// GetKabaneroNamespace Get namespace of where kabanero is installed
//func GetKabaneroNamespace() string {
//	if kabaneroNamespace == "" {
//		kabaneroNamespace = os.Getenv(KUBENAMESPACE)
//		if kabaneroNamespace == "" {
//			kabaneroNamespace = DEFAULTNAMESPACE
//		}
//	}
//
//	return kabaneroNamespace
//}

// GetTriggerFiles returns the directory containing the retrieved trigger files.
//func GetTriggerFiles(client rest.Interface, url *url.URL, skipChkSumVerify bool) (string, error) {
//	/* Get namespace of where kabanero is installed and the kabanero index URL */
//	webhookNamespace := GetKabaneroNamespace()
//	var triggerChkSum string
//	var err error
//
//	/* Use the trigger URL from the Kabanero CR if none was set */
//	if url == nil {
//		url, triggerChkSum, err = GetTriggerInfo(client, webhookNamespace)
//		if err != nil {
//			klog.Fatal(err)
//		}
//	}
//
//	/* Use a local directory if no scheme was provided or if it's set to file. */
//	if url.Scheme == "" || url.Scheme == "file" {
//		return url.Path, nil
//	}
//
//	/* Otherwise create a temporary directory and try to download/unpack the trigger files there. */
//	triggerDir, err := ioutil.TempDir("", "webhook")
//	if err != nil {
//		return "", fmt.Errorf("unable to create temproary directory: %v", err)
//	}
//
//	err = DownloadTrigger(url.String(), triggerChkSum, triggerDir, !skipChkSumVerify)
//	if err != nil {
//		return "", fmt.Errorf("unable to download trigger archive pointed by URL at %s: %v", url, err)
//	}
//
//	return triggerDir, err
//}

// GetTriggerInfo Get the URL to trigger gzipped tar and its sha256 checksum.
func GetTriggerInfo(client rest.Interface, namespace string) (*url.URL, string, error) {
	kabaneroList := kabanerov1alpha2.KabaneroList{}
	err := client.Get().Resource(KABANEROS).Namespace(namespace).Do().Into(&kabaneroList)
	if err != nil {
		return nil, "", err
	}

	for _, kabanero := range kabaneroList.Items {
		if klog.V(1) {
			klog.Infof("Checking for trigger URL in kabanero/%s", kabanero.Name)
		}

		for _, triggerSpec := range kabanero.Spec.Triggers {
			if klog.V(1) {
				klog.Infof("Success. Found trigger '%s' (checksum: %s) -> %s", triggerSpec.Id, triggerSpec.Sha256, triggerSpec.Https.Url)
			}
			if triggerSpec.Https.Url != "" {
				url, err := url.Parse(triggerSpec.Https.Url)
				return url, triggerSpec.Sha256, err
			}
		}
	}

	return nil, "", fmt.Errorf("unable to find trigger URL in any kabanero definition")
}

/*
GetGitHubSecret Find the user/token for a GitHub API key. The format of the secret:
apiVersion: v1
kind: Secret
metadata:
  name: gh-https-secret
  annotations:
    tekton.dev/git-0: https://github.com
type: kubernetes.io/basic-auth
stringData:
  username: <username>
  password: <token>

This will scan for a secret with either of the following annotations:
 * tekton.dev/git-*
 * kabanero.io/git-*

GetGitHubSecret will return the username and token of a secret whose annotation's value is a prefix match for repoURL.
Note that a secret with the `kabanero.io/git-*` annotation is preferred over one with `tekton.dev/git-*`.
Return: username, token, error
*/
func GetGitHubSecret(kubeClient client.Client, namespace string, repoURL string) (string, string, error) {
	// TODO: Change to controller pattern and cache the secrets.
	if klog.V(8) {
		klog.Infof("GetGitHubSecret namespace: %s, repoURL: %s", namespace, repoURL)
	}

    secrets := &corev1.SecretList{}
    options := []client.ListOption{client.InNamespace(namespace)}
    err := kubeClient.List(context.Background(), secrets, options...)
	if err != nil {
		return "", "", err
	}

	secret := getGitHubSecretForRepo(secrets, repoURL)
	if secret == nil {
		return "", "", fmt.Errorf("unable to find GitHub token for url: %s", repoURL)
	}

	username, ok := secret.Data["username"]
	if !ok {
		return "", "", fmt.Errorf("unable to find username field of secret: %s", secret.Name)
	}

	token, ok := secret.Data["password"]
	if !ok {
		return "", "", fmt.Errorf("unable to find password field of secret: %s", secret.Namespace)
	}

	return string(username), string(token), nil
}

func getGitHubSecretForRepo(secrets *corev1.SecretList, repoURL string) *corev1.Secret {
	var tknSecret *corev1.Secret
	for i, secret := range secrets.Items {
		for key, val := range secret.Annotations {
			if strings.HasPrefix(key, "tekton.dev/git-") && strings.HasPrefix(repoURL, val) {
				tknSecret = &secrets.Items[i]
			} else if strings.HasPrefix(key, "kabanero.io/git-") && strings.HasPrefix(repoURL, val) {
				// Since we prefer the kabanero.io annotation, we can terminate early if we find one that matches.
				return &secrets.Items[i]
			}
		}
	}

	return tknSecret
}

/*
 Input:
	str: input string
	arrStr: input array of string
 Return:
	true if any element of arrStr is a prefix of str
	the first element of arrStr that is a prefix of str
*/
func matchPrefix(str string, arrStr []string) (bool, string) {
	for _, val := range arrStr {
		if strings.HasPrefix(str, val) {
			return true, val
		}
	}
	return false, ""
}

func  imageMatches(repoStackImage string,  images []kabanerov1alpha2.ImageStatus) bool  {
    for _, image := range images {
         if image.Image == repoStackImage {
             return true
         }
    }
    return false
}


func findEventListener(versionStatus *kabanerov1alpha2.StackVersionStatus) (string, string) {
    for _, pipeline := range versionStatus.Pipelines {
        for _, activeAsset := range pipeline.ActiveAssets {
             if activeAsset.Group == TEKTON_DEV && activeAsset.Kind == EVENT_LISTENER {
                  /* found */
                  return activeAsset.Namespace, activeAsset.Name
             }
        }
    }
    return "", ""
}

/* Find the Kabanero Tekton event listener for stack 
input:
   kubeClient: client to API server
   namespace: namespace of stack to search for event listener
   repoStackImage:  the name  of image as specified in .appsody-config.yaml. For example, "docker.io/appsody/nodejs:0.3"
   repoStackVersion: the semantic version for the stack, as specified in .appsody-config.yaml. For example "0.3"
Return:
   name of listener, or "" if no match
   exact version found
   error : if any error occurred when matching the repository to an event listener
*/
func FindEventListenerForStack(kubeClient client.Client, namespace string, repoStackImage string, repoStackVersion string) (string, string, error) {
//    if true {
//        return "http://el-listener-mcheng.tekton-pipelines.svc.cluster.local:8080", "0.2.0", nil
//    }

	if klog.V(8) {
		klog.Infof("FindEventListenerForStack namespace: %s, reposStackImage: %v, repoStackVersion: %v", namespace, repoStackImage, repoStackVersion)
	}
    repoVersion, err := semverimage.NewVersion(repoStackVersion)
    if err != nil {
        return "", "", err
    }
    stacks := &kabanerov1alpha2.StackList{}
    options := []client.ListOption{client.InNamespace(namespace)}
    err = kubeClient.List(context.Background(), stacks, options...) 
    if err != nil {
		return "", "", err
	}

    currentListener := ""
    currentNamespace := ""
    currentVersion, _ := semverimage.NewVersion("0.0.0")
    for _, stack  := range stacks.Items {
        status := stack.Status
        for _, versionStatus := range status.Versions {
           if versionStatus.Status != ACTIVE  {
                continue
           }
           if  !imageMatches(repoStackImage, versionStatus.Images)  {
               continue
           }
           matchedVersion, err := semverimage.NewVersion(versionStatus.Version)
           if err != nil {
                return "", "", err
           }
           matched := repoVersion.IsCompatible(matchedVersion)
           if !matched {
                continue
           }
           matchedNamespace, matchedListener := findEventListener(&versionStatus)
           if  matchedListener == "" {
                 continue
           }
           if currentListener == "" || matchedVersion.GreaterThan(currentVersion) {
               currentListener = matchedListener
               currentNamespace = matchedNamespace
               currentVersion = matchedVersion
           }
        }
    }

    if currentListener == "" {
        klog.Errorf("Unable to find listener from stack for appsody repo %v:%v", repoStackImage, repoStackVersion)
        return currentListener, currentVersion.String(), nil
    }

    /* Find the actual listener */
    listeners := &triggers.EventListenerList{}
    options = []client.ListOption{client.InNamespace(currentNamespace)}
    err = kubeClient.List(context.Background(), listeners, options...) 
    if err != nil {
        klog.Errorf("Unable to find listener %v in namespace %v, error: %v", currentListener, currentNamespace, err)
		return "", "", err
	}
    for _, listener := range listeners.Items {
        klog.Infof("Processing listener %v in namespace %v, looking for: %v", listener.Name, currentNamespace, currentListener)
        if listener.Name == currentListener {
             /* found */
             if listener.Status.Address == nil || listener.Status.Address.URL == nil {
                 klog.Errorf("Found listener %v in namespace %v, but no URL status: %v", currentListener, currentNamespace, err)
                 return "", currentVersion.String(), fmt.Errorf("found listener %v in namespace %v, but no URL status %v", currentListener, currentNamespace, err)
             }
             return listener.Status.Address.URL.String(), currentVersion.String(), nil
        }
    }

    /* not found */
    klog.Errorf("Unable to find listener %v in namespace %v", currentListener, currentNamespace)
    return "", currentVersion.String(), nil
}

