/*
 (c) Copyright [2021] Micro Focus or one of its affiliates.
 Licensed under the Apache License, Version 2.0 (the "License");
 You may not use this file except in compliance with the License.
 You may obtain a copy of the License at

 http://www.apache.org/licenses/LICENSE-2.0

 Unless required by applicable law or agreed to in writing, software
 distributed under the License is distributed on an "AS IS" BASIS,
 WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 See the License for the specific language governing permissions and
 limitations under the License.
*/

package controllers

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	vapi "github.com/vertica/vertica-kubernetes/api/v1beta1"
	"github.com/vertica/vertica-kubernetes/pkg/atconf"
	"github.com/vertica/vertica-kubernetes/pkg/cmds"
	"github.com/vertica/vertica-kubernetes/pkg/paths"
	ctrl "sigs.k8s.io/controller-runtime"
)

var _ = Describe("k8s/uninstall_reconcile", func() {
	ctx := context.Background()

	It("reconcile subcluster should not return an error if the sts doesn't exist", func() {
		vdb := vapi.MakeVDB()

		fpr := &cmds.FakePodRunner{}
		pfacts := MakePodFacts(k8sClient, fpr)
		recon := MakeUninstallReconciler(vrec, logger, vdb, fpr, &pfacts)
		Expect(recon.Reconcile(ctx, &ctrl.Request{})).Should(Equal(ctrl.Result{}))
	})

	It("should uninstall one pod", func() {
		vdb := vapi.MakeVDB()
		sc := &vdb.Spec.Subclusters[0]
		sc.Size = 2
		createPods(ctx, vdb, AllPodsRunning)
		defer deletePods(ctx, vdb)

		fpr := &cmds.FakePodRunner{}
		pfacts := MakePodFacts(k8sClient, fpr)
		actor := MakeUninstallReconciler(vrec, logger, vdb, fpr, &pfacts)
		recon := actor.(*UninstallReconciler)
		recon.ATWriter = &atconf.FakeWriter{}
		Expect(pfacts.Collect(ctx, vdb)).Should(Succeed())
		_, err := recon.uninstallPodsInSubcluster(ctx, sc, 1, 1)
		Expect(err).Should(Succeed())
		rmIndCmd := fpr.FindCommands(fmt.Sprintf("rm %s", paths.InstallerIndicatorFile))
		Expect(len(rmIndCmd)).Should(Equal(1))
	})

	It("should skip uninstall and requeue because there aren't any pods running", func() {
		vdb := vapi.MakeVDB()
		sc := &vdb.Spec.Subclusters[0]
		sc.Size = 2
		vdbCopy := vdb.DeepCopy() // Take a copy so that cleanup with original size
		createPods(ctx, vdb, AllPodsNotRunning)
		defer deletePods(ctx, vdbCopy)
		sc.Size = 1 // Set to 1 to mimic a pending uninstall

		fpr := &cmds.FakePodRunner{}
		pfacts := MakePodFacts(k8sClient, fpr)
		r := MakeUninstallReconciler(vrec, logger, vdb, fpr, &pfacts)
		res, err := r.Reconcile(ctx, &ctrl.Request{})
		Expect(err).Should(Succeed())
		Expect(res.Requeue).Should(BeTrue())
		Expect(len(fpr.Histories)).Should(Equal(0))
	})

	It("should call uninstall for multiple pods", func() {
		vdb := vapi.MakeVDB()
		sc := &vdb.Spec.Subclusters[0]
		sc.Size = 3
		vdbCopy := vdb.DeepCopy() // Take a copy so that we cleanup with the original size
		createPods(ctx, vdb, AllPodsRunning)
		defer deletePods(ctx, vdbCopy)
		sc.Size = 1 // mimic a pending db_remove_node

		fpr := &cmds.FakePodRunner{}
		pfacts := MakePodFacts(k8sClient, fpr)
		actor := MakeUninstallReconciler(vrec, logger, vdb, fpr, &pfacts)
		r := actor.(*UninstallReconciler)
		r.ATWriter = &atconf.FakeWriter{}
		res, err := r.Reconcile(ctx, &ctrl.Request{})
		Expect(err).Should(Succeed())
		Expect(res.Requeue).Should(BeFalse())
		rmIndCmd := fpr.FindCommands(fmt.Sprintf("rm %s", paths.InstallerIndicatorFile))
		Expect(len(rmIndCmd)).Should(Equal(2))
	})
})
