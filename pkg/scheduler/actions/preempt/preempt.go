/*
Copyright 2018 The Kubernetes Authors.

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

package preempt

import (
	"github.com/golang/glog"

	"github.com/kubernetes-incubator/kube-arbitrator/pkg/scheduler/api"
	"github.com/kubernetes-incubator/kube-arbitrator/pkg/scheduler/framework"
	"github.com/kubernetes-incubator/kube-arbitrator/pkg/scheduler/util"
)

type preemptAction struct {
	ssn *framework.Session
}

func New() *preemptAction {
	return &preemptAction{}
}

func (alloc *preemptAction) Name() string {
	return "preempt"
}

func (alloc *preemptAction) Initialize() {}

func (alloc *preemptAction) Execute(ssn *framework.Session) {
	glog.V(3).Infof("Enter Preempt ...")
	defer glog.V(3).Infof("Leaving Preempt ...")

	jobRevOrderFn := func(l, r interface{}) bool {
		return !ssn.JobOrderFn(l, r)
	}

	taskRevOrderFn := func(l, r interface{}) bool {
		return !ssn.TaskOrderFn(l, r)
	}

	preemptors := util.NewPriorityQueue(ssn.JobOrderFn)
	preemptees := util.NewPriorityQueue(jobRevOrderFn)

	preemptorTasks := map[api.JobID]*util.PriorityQueue{}
	preempteeTasks := map[api.JobID]*util.PriorityQueue{}

	for _, job := range ssn.Jobs {
		preemptors.Push(job)
		preemptorTasks[job.UID] = util.NewPriorityQueue(ssn.TaskOrderFn)
		for _, task := range job.TaskStatusIndex[api.Pending] {
			preemptorTasks[job.UID].Push(task)
		}

		// If no running tasks in job, skip it as preemptee.
		if len(job.TaskStatusIndex[api.Running]) != 0 {
			preemptees.Push(job)
			preempteeTasks[job.UID] = util.NewPriorityQueue(taskRevOrderFn)
			// TODO (k82cn): it's better to also includes Binding/Bound tasks.
			for _, task := range job.TaskStatusIndex[api.Running] {
				preempteeTasks[job.UID].Push(task)
			}
		}
	}

	for {
		// If no preemptors nor preemptees, no preemption.
		if preemptors.Empty() || preemptees.Empty() {
			break
		}

		preemptorJob := preemptors.Pop().(*api.JobInfo)

		// If not preemptor tasks, next job.
		if preemptorTasks[preemptorJob.UID].Empty() {
			continue
		}

		preempteeJob := preemptees.Pop().(*api.JobInfo)
		for preempteeTasks[preempteeJob.UID].Empty() && preemptorJob.UID != preempteeJob.UID {
			preempteeJob = preemptees.Pop().(*api.JobInfo)
		}

		// The most underused job can not preempt any resource, break the loop.
		if preemptorJob.UID == preempteeJob.UID {
			break
		}

		glog.V(3).Infof("The preemptor is %v:%v/%v, the preemptee is %v:%v/%v",
			preemptorJob.UID, preemptorJob.Namespace, preemptorJob.Name,
			preempteeJob.UID, preempteeJob.Namespace, preempteeJob.Name)

		preemptor := preemptorTasks[preemptorJob.UID].Pop().(*api.TaskInfo)
		preemptee := preempteeTasks[preempteeJob.UID].Pop().(*api.TaskInfo)

		preempted := false

		if ssn.Preemptable(preemptor, preemptee) {
			if err := ssn.Preempt(preemptor, preemptee); err != nil {
				glog.Errorf("Failed to evict task %v for task %v: %v", nil, nil, err)
			} else {
				preempted = true
			}
		} else {
			glog.V(3).Infof("Can not preempt task <%v:%v/%v> for task <%v:%v/%v>",
				preemptee.UID, preemptee.Namespace, preemptee.Name,
				preemptor.UID, preemptor.Namespace, preemptor.Name)
		}

		// If preempted resource, put it back to the queue.
		if preempted {
			preemptors.Push(preemptorJob)
		} else {
			// If the preemptee is not preempted, push it back for other to preempt.
			preempteeTasks[preempteeJob.UID].Push(preemptee)
		}

		preemptees.Push(preempteeJob)
	}
}

func (alloc *preemptAction) UnInitialize() {}
