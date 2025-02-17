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

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	vapi "github.com/vertica/vertica-kubernetes/api/v1beta1"
	"github.com/vertica/vertica-kubernetes/pkg/cmds"
	"github.com/vertica/vertica-kubernetes/pkg/names"
	ctrl "sigs.k8s.io/controller-runtime"
)

var _ = Describe("k8s/version_reconcile", func() {
	ctx := context.Background()

	It("parsing of version output should return expected annotations", func() {
		vdb := vapi.MakeVDB()
		fpr := &cmds.FakePodRunner{}
		pfacts := MakePodFacts(k8sClient, fpr)
		act := MakeVersionReconciler(vrec, logger, vdb, fpr, &pfacts)
		r := act.(*VersionReconciler)
		op := `Vertica Analytic Database v11.0.0-20210601
vertica(v11.0.0-20210601) built by @re-docker2 from master@da8f0e93f1ee720d8e4f8e1366a26c0d9dd7f9e7 on 'Tue Jun  1 05:04:35 2021' $BuildId$`
		ans := r.parseVersionOutput(op)
		const NumAnnotations = 3
		Expect(len(ans)).Should(Equal(NumAnnotations))
		Expect(ans[vapi.VersionAnnotation]).Should(Equal("v11.0.0-20210601"))
		Expect(ans[vapi.BuildDateAnnotation]).Should(Equal("Tue Jun  1 05:04:35 2021"))
		Expect(ans[vapi.BuildRefAnnotation]).Should(Equal("da8f0e93f1ee720d8e4f8e1366a26c0d9dd7f9e7"))
	})

	It("should indicate no change if annotations stayed the same", func() {
		vdb := vapi.MakeVDB()
		vdb.ObjectMeta.Annotations = map[string]string{
			vapi.BuildDateAnnotation: "Tue Jun 10",
			vapi.BuildRefAnnotation:  "abcd",
			vapi.VersionAnnotation:   "v11.0.0",
		}

		fpr := &cmds.FakePodRunner{}
		pfacts := MakePodFacts(k8sClient, fpr)
		act := MakeVersionReconciler(vrec, logger, vdb, fpr, &pfacts)
		r := act.(*VersionReconciler)
		op := `Vertica Analytic Database v11.0.0
vertica(v11.0.0) built by @re-docker2 from master@abcd on 'Tue Jun 10' $BuildId$
`
		chg := r.mergeAnnotations(r.parseVersionOutput(op))
		Expect(chg).Should(BeFalse())
	})

	It("should indicate a change if annotations changed", func() {
		vdb := vapi.MakeVDB()
		vdb.ObjectMeta.Annotations = map[string]string{
			vapi.BuildDateAnnotation: "Tue Jun 10",
			vapi.BuildRefAnnotation:  "abcd",
			vapi.VersionAnnotation:   "v10.1.1-0",
		}

		fpr := &cmds.FakePodRunner{}
		pfacts := MakePodFacts(k8sClient, fpr)
		act := MakeVersionReconciler(vrec, logger, vdb, fpr, &pfacts)
		r := act.(*VersionReconciler)
		op := `Vertica Analytic Database v11.0.0-1
vertica(v11.0.0-1) built by @re-docker2 from master@abcd on 'Tue Jun 10' $BuildId$
`
		chg := r.mergeAnnotations(r.parseVersionOutput(op))
		Expect(chg).Should(BeTrue())

		vdb.ObjectMeta.Annotations = map[string]string{
			vapi.BuildDateAnnotation: "Tue Jun 10",
		}
		chg = r.mergeAnnotations(r.parseVersionOutput(op))
		Expect(chg).Should(BeTrue())
	})

	It("should update annotations in vdb since they differ", func() {
		vdb := vapi.MakeVDB()
		vdb.Spec.Subclusters[0].Size = 1
		createVdb(ctx, vdb)
		defer deleteVdb(ctx, vdb)
		createPods(ctx, vdb, AllPodsRunning)
		defer deletePods(ctx, vdb)

		fpr := &cmds.FakePodRunner{}
		pfacts := MakePodFacts(k8sClient, fpr)
		Expect(pfacts.Collect(ctx, vdb)).Should(Succeed())
		podName := names.GenPodName(vdb, &vdb.Spec.Subclusters[0], 0)
		fpr.Results = cmds.CmdResults{
			podName: []cmds.CmdResult{
				{
					Stdout: `Vertica Analytic Database v10.1.1-0
vertica(v11.1.0) built by @re-docker2 from tag@releases/VER_10_1_RELEASE_BUILD_10_20210413 on 'Wed Jun  2 2021' $BuildId$
`,
				},
			},
		}
		r := MakeVersionReconciler(vrec, logger, vdb, fpr, &pfacts)
		Expect(r.Reconcile(ctx, &ctrl.Request{})).Should(Equal(ctrl.Result{}))

		fetchVdb := &vapi.VerticaDB{}
		Expect(k8sClient.Get(ctx, vapi.MakeVDBName(), fetchVdb)).Should(Succeed())
		Expect(len(fetchVdb.ObjectMeta.Annotations)).Should(Equal(3))
		Expect(fetchVdb.ObjectMeta.Annotations[vapi.VersionAnnotation]).Should(Equal("v10.1.1-0"))
		Expect(fetchVdb.ObjectMeta.Annotations[vapi.BuildRefAnnotation]).Should(Equal("releases/VER_10_1_RELEASE_BUILD_10_20210413"))
		Expect(fetchVdb.ObjectMeta.Annotations[vapi.BuildDateAnnotation]).Should(Equal("Wed Jun  2 2021"))
	})
})
