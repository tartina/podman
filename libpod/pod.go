package libpod

import (
	"time"

	"github.com/containers/storage"
	"github.com/pkg/errors"
)

var (
	// KernelNamespaces is a list of the kernel namespaces a pod can share
	KernelNamespaces = []string{"ipc", "net", "pid", "user", "mnt", "uts", "cgroup"}
)

// Pod represents a group of containers that are managed together.
// Any operations on a Pod that access state must begin with a call to
// updatePod().
// There is no guarantee that state exists in a readable state before this call,
// and even if it does its contents will be out of date and must be refreshed
// from the database.
// Generally, this requirement applies only to top-level functions; helpers can
// assume their callers handled this requirement. Generally speaking, if a
// function takes the pod lock and accesses any part of state, it should
// updatePod() immediately after locking.
// ffjson: skip
// Pod represents a group of containers that may share namespaces
type Pod struct {
	config *PodConfig
	state  *podState

	valid   bool
	runtime *Runtime
	lock    storage.Locker
}

// PodConfig represents a pod's static configuration
type PodConfig struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// Namespace the pod is in
	Namespace string `json:"namespace,omitempty"`

	// Labels contains labels applied to the pod
	Labels map[string]string `json:"labels"`
	// CgroupParent contains the pod's CGroup parent
	CgroupParent string `json:"cgroupParent"`

	// UsePodCgroup indicates whether the pod will create its own CGroup and
	// join containers to it.
	// If true, all containers joined to the pod will use the pod cgroup as
	// their cgroup parent, and cannot set a different cgroup parent
	UsePodCgroup bool `json:"sharesCgroup,omitempty"`

	// The following UsePod{kernelNamespace} indicate whether the containers
	// in the pod will inherit the namespace from the first container in the pod.
	UsePodPID  bool `json:"sharesPid,omitempty"`
	UsePodIPC  bool `json:"sharesIpc,omitempty"`
	UsePodNet  bool `json:"sharesNet,omitempty"`
	UsePodMNT  bool `json:"sharesMnt,omitempty"`
	UsePodUser bool `json:"sharesUser,omitempty"`
	UsePodUTS  bool `json:"sharesUts,omitempty"`

	PauseContainer *PauseContainerConfig `json:"pauseConfig"`

	// Time pod was created
	CreatedTime time.Time `json:"created"`
}

// podState represents a pod's state
type podState struct {
	// CgroupPath is the path to the pod's CGroup
	CgroupPath string `json:"cgroupPath"`
	// PauseContainerID is the container that holds pod namespace information
	// Most often a pause container
	PauseContainerID string
}

// PodInspect represents the data we want to display for
// podman pod inspect
type PodInspect struct {
	Config     *PodConfig
	State      *PodInspectState
	Containers []PodContainerInfo
}

// PodInspectState contains inspect data on the pod's state
type PodInspectState struct {
	CgroupPath       string `json:"cgroupPath"`
	PauseContainerID string `json:"pauseContainerID"`
}

// PodContainerInfo keeps information on a container in a pod
type PodContainerInfo struct {
	ID    string `json:"id"`
	State string `json:"state"`
}

// PauseContainerConfig is the configuration for the pod's pause container
type PauseContainerConfig struct {
	HasPauseContainer bool `json:"makePauseContainer"`
}

// ID retrieves the pod's ID
func (p *Pod) ID() string {
	return p.config.ID
}

// Name retrieves the pod's name
func (p *Pod) Name() string {
	return p.config.Name
}

// Namespace returns the pod's libpod namespace.
// Namespaces are used to logically separate containers and pods in the state.
func (p *Pod) Namespace() string {
	return p.config.Namespace
}

// Labels returns the pod's labels
func (p *Pod) Labels() map[string]string {
	labels := make(map[string]string)
	for key, value := range p.config.Labels {
		labels[key] = value
	}

	return labels
}

// CreatedTime gets the time when the pod was created
func (p *Pod) CreatedTime() time.Time {
	return p.config.CreatedTime
}

// CgroupParent returns the pod's CGroup parent
func (p *Pod) CgroupParent() string {
	return p.config.CgroupParent
}

// SharesPID returns whether containers in pod
// default to use PID namespace of first container in pod
func (p *Pod) SharesPID() bool {
	return p.config.UsePodPID
}

// SharesIPC returns whether containers in pod
// default to use IPC namespace of first container in pod
func (p *Pod) SharesIPC() bool {
	return p.config.UsePodIPC
}

// SharesNet returns whether containers in pod
// default to use network namespace of first container in pod
func (p *Pod) SharesNet() bool {
	return p.config.UsePodNet
}

// SharesMNT returns whether containers in pod
// default to use PID namespace of first container in pod
func (p *Pod) SharesMNT() bool {
	return p.config.UsePodMNT
}

// SharesUser returns whether containers in pod
// default to use user namespace of first container in pod
func (p *Pod) SharesUser() bool {
	return p.config.UsePodUser
}

// SharesUTS returns whether containers in pod
// default to use UTS namespace of first container in pod
func (p *Pod) SharesUTS() bool {
	return p.config.UsePodUTS
}

// SharesCgroup returns whether containers in the pod will default to this pod's
// cgroup instead of the default libpod parent
func (p *Pod) SharesCgroup() bool {
	return p.config.UsePodCgroup
}

// CgroupPath returns the path to the pod's CGroup
func (p *Pod) CgroupPath() (string, error) {
	p.lock.Lock()
	defer p.lock.Unlock()
	if err := p.updatePod(); err != nil {
		return "", err
	}

	return p.state.CgroupPath, nil
}

// HasContainer checks if a container is present in the pod
func (p *Pod) HasContainer(id string) (bool, error) {
	if !p.valid {
		return false, ErrPodRemoved
	}

	return p.runtime.state.PodHasContainer(p, id)
}

// AllContainersByID returns the container IDs of all the containers in the pod
func (p *Pod) AllContainersByID() ([]string, error) {
	p.lock.Lock()
	defer p.lock.Unlock()

	if !p.valid {
		return nil, ErrPodRemoved
	}

	return p.runtime.state.PodContainersByID(p)
}

// AllContainers retrieves the containers in the pod
func (p *Pod) AllContainers() ([]*Container, error) {
	if !p.valid {
		return nil, ErrPodRemoved
	}
	p.lock.Lock()
	defer p.lock.Unlock()
	return p.allContainers()
}

func (p *Pod) allContainers() ([]*Container, error) {
	return p.runtime.state.PodContainers(p)
}

// HasPauseContainer returns whether the pod will create a pause container
func (p *Pod) HasPauseContainer() bool {
	return p.config.PauseContainer.HasPauseContainer
}

// SharesNamespaces checks if the pod has any kernel namespaces set as shared. A pause container will not be
// created if no kernel namespaces are shared.
func (p *Pod) SharesNamespaces() bool {
	return p.SharesPID() || p.SharesIPC() || p.SharesNet() || p.SharesMNT() || p.SharesUser() || p.SharesUTS()
}

// PauseContainerID returns a the pause container ID for a pod.
// If the container returned is "", the pod has no pause container.
func (p *Pod) PauseContainerID() (string, error) {
	p.lock.Lock()
	defer p.lock.Unlock()

	if err := p.updatePod(); err != nil {
		return "", err
	}

	return p.state.PauseContainerID, nil
}

// TODO add pod batching
// Lock pod to avoid lock contention
// Store and lock all containers (no RemoveContainer in batch guarantees cache will not become stale)

// PodContainerStats is an organization struct for pods and their containers
type PodContainerStats struct {
	Pod            *Pod
	ContainerStats map[string]*ContainerStats
}

// GetPodStats returns the stats for each of its containers
func (p *Pod) GetPodStats(previousContainerStats map[string]*ContainerStats) (map[string]*ContainerStats, error) {
	var (
		ok       bool
		prevStat *ContainerStats
	)
	p.lock.Lock()
	defer p.lock.Unlock()

	if err := p.updatePod(); err != nil {
		return nil, err
	}
	containers, err := p.runtime.state.PodContainers(p)
	if err != nil {
		return nil, err
	}
	newContainerStats := make(map[string]*ContainerStats)
	for _, c := range containers {
		if prevStat, ok = previousContainerStats[c.ID()]; !ok {
			prevStat = &ContainerStats{}
		}
		newStats, err := c.GetContainerStats(prevStat)
		// If the container wasn't running, don't include it
		// but also suppress the error
		if err != nil && errors.Cause(err) != ErrCtrStateInvalid {
			return nil, err
		}
		if err == nil {
			newContainerStats[c.ID()] = newStats
		}
	}
	return newContainerStats, nil
}
