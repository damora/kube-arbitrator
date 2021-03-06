/*
Copyright 2017 The Kubernetes Authors.

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

package api

import (
	"github.com/golang/glog"

	"k8s.io/api/core/v1"
)

// NodeInfo is node level aggregated information.
type NodeInfo struct {
	Name string
	Node *v1.Node

	// The releasing resource on that node
	Releasing *Resource
	// The idle resource on that node
	Idle *Resource
	// The used resource on that node, including running and terminating
	// pods
	Used *Resource

	Allocatable *Resource
	Capability  *Resource

	Tasks map[TaskID]*TaskInfo
}

func NewNodeInfo(node *v1.Node) *NodeInfo {
	if node == nil {
		return &NodeInfo{
			Releasing: EmptyResource(),
			Idle:      EmptyResource(),
			Used:      EmptyResource(),

			Allocatable: EmptyResource(),
			Capability:  EmptyResource(),

			Tasks: make(map[TaskID]*TaskInfo),
		}
	}

	return &NodeInfo{
		Name: node.Name,
		Node: node,

		Releasing: EmptyResource(),
		Idle:      NewResource(node.Status.Allocatable),
		Used:      EmptyResource(),

		Allocatable: NewResource(node.Status.Allocatable),
		Capability:  NewResource(node.Status.Capacity),

		Tasks: make(map[TaskID]*TaskInfo),
	}
}

func (ni *NodeInfo) Clone() *NodeInfo {
	pods := make(map[TaskID]*TaskInfo, len(ni.Tasks))

	for _, p := range ni.Tasks {
		pods[PodKey(p.Pod)] = p.Clone()
	}

	return &NodeInfo{
		Name:        ni.Name,
		Node:        ni.Node,
		Idle:        ni.Idle.Clone(),
		Used:        ni.Used.Clone(),
		Releasing:   ni.Releasing.Clone(),
		Allocatable: ni.Allocatable.Clone(),
		Capability:  ni.Capability.Clone(),

		Tasks: pods,
	}
}

func (ni *NodeInfo) SetNode(node *v1.Node) {
	if ni.Node == nil {
		ni.Idle = NewResource(node.Status.Allocatable)

		for _, task := range ni.Tasks {
			if task.Status == Releasing {
				ni.Releasing.Add(task.Resreq)
			}

			ni.Idle.Sub(task.Resreq)
			ni.Used.Add(task.Resreq)
		}
	}

	ni.Name = node.Name
	ni.Node = node
	ni.Allocatable = NewResource(node.Status.Allocatable)
	ni.Capability = NewResource(node.Status.Capacity)
}

func (ni *NodeInfo) PipelineTask(task *TaskInfo) {
	key := PodKey(task.Pod)
	if _, found := ni.Tasks[key]; found {
		glog.Errorf("Task <%v/%v> already on node <%v>, should not add again.",
			task.Namespace, task.Name, ni.Name)
		return
	}

	if ni.Node != nil {
		ni.Releasing.Sub(task.Resreq)
		ni.Used.Add(task.Resreq)
	}

	ni.Tasks[key] = task
}

func (ni *NodeInfo) AddTask(task *TaskInfo) {
	key := PodKey(task.Pod)
	if _, found := ni.Tasks[key]; found {
		glog.Errorf("Task <%v/%v> already on node <%v>, should not add again.",
			task.Namespace, task.Name, ni.Name)
		return
	}

	if ni.Node != nil {
		if task.Status == Releasing {
			ni.Releasing.Add(task.Resreq)
		}
		ni.Idle.Sub(task.Resreq)
		ni.Used.Add(task.Resreq)
	}

	glog.V(3).Infof("After added Task <%v> from Node <%v>: idle <%v>, used <%v>, releasing <%v>",
		key, ni.Name, ni.Idle, ni.Used, ni.Releasing)

	ni.Tasks[key] = task
}

func (ni *NodeInfo) RemoveTask(ti *TaskInfo) {
	key := PodKey(ti.Pod)

	task, found := ni.Tasks[key]
	if !found {
		return
	}

	if ni.Node != nil {
		if task.Status == Releasing {
			ni.Releasing.Sub(task.Resreq)
		}

		ni.Idle.Add(task.Resreq)
		ni.Used.Sub(task.Resreq)
	}

	glog.V(3).Infof("After removed Task <%v> from Node <%v>: idle <%v>, used <%v>, releasing <%v>",
		key, ni.Name, ni.Idle, ni.Used, ni.Releasing)

	delete(ni.Tasks, key)
}
