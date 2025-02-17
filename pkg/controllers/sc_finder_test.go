/*
 (c) Copyright [2018-2021] Micro Focus or one of its affiliates.
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
	"github.com/vertica/vertica-kubernetes/pkg/names"
)

var _ = Describe("sc_finder", func() {
	ctx := context.Background()

	It("should find all subclusters that exist in the vdb", func() {
		vdb := vapi.MakeVDB()
		scNames := []string{"sc1", "sc2", "sc3"}
		scSizes := []int32{10, 5, 8}
		vdb.Spec.Subclusters = []vapi.Subcluster{
			{Name: scNames[0], Size: scSizes[0]},
			{Name: scNames[1], Size: scSizes[1]},
			{Name: scNames[2], Size: scSizes[2]},
		}

		finder := MakeSubclusterFinder(k8sClient, vdb)
		scs, err := finder.FindSubclusters(ctx, FindInVdb)
		Expect(err).Should(Succeed())
		Expect(len(scs)).Should(Equal(len(scNames)))
		for i := 0; i < len(scNames); i++ {
			Expect(scs[i].Name).Should(Equal(scNames[i]))
			Expect(scs[i].Size).Should(Equal(scSizes[i]))
		}
	})

	It("should find subclusters that don't exist in the vdb", func() {
		vdb := vapi.MakeVDB()
		scNames := []string{"sc1", "sc2"}
		scSizes := []int32{10, 5}
		vdb.Spec.Subclusters = []vapi.Subcluster{
			{Name: scNames[0], Size: scSizes[0]},
			{Name: scNames[1], Size: scSizes[1]},
		}
		createPods(ctx, vdb, AllPodsRunning)
		defer deletePods(ctx, vdb)

		// We create a second vdb without one of the subclusters.  We then use
		// the finder to discover this additional subcluster.
		lookupVdb := vapi.MakeVDB()
		lookupVdb.Spec.Subclusters[0] = vapi.Subcluster{Name: scNames[0], Size: scSizes[0]}

		finder := MakeSubclusterFinder(k8sClient, lookupVdb)
		scs, err := finder.FindSubclusters(ctx, FindAll)
		Expect(err).Should(Succeed())
		Expect(len(scs)).Should(Equal(len(scNames)))
		for i := 0; i < len(scNames); i++ {
			Expect(scs[i].Name).Should(Equal(scNames[i]))
		}
	})

	It("should find statefulsets that don't exist in the vdb", func() {
		vdb := vapi.MakeVDB()
		scNames := []string{"sc1", "sc2"}
		scSizes := []int32{10, 5}
		vdb.Spec.Subclusters = []vapi.Subcluster{
			{Name: scNames[0], Size: scSizes[0]},
			{Name: scNames[1], Size: scSizes[1]},
		}
		createPods(ctx, vdb, AllPodsRunning)
		defer deletePods(ctx, vdb)

		// We create a second vdb without one of the subclusters.  We then use
		// the finder to discover this additional subcluster.
		lookupVdb := vapi.MakeVDB()
		lookupVdb.Spec.Subclusters[0] = vapi.Subcluster{Name: scNames[1], Size: scSizes[1]}

		finder := MakeSubclusterFinder(k8sClient, lookupVdb)
		sts, err := finder.FindStatefulSets(ctx, FindNotInVdb)
		Expect(err).Should(Succeed())
		Expect(len(sts.Items)).Should(Equal(1))
		Expect(sts.Items[0].Name).Should(Equal(names.GenStsName(vdb, &vdb.Spec.Subclusters[0]).Name))
	})

	It("should find service objects that exist in the vdb", func() {
		vdb := vapi.MakeVDB()
		sc := &vdb.Spec.Subclusters[0]
		createSvcs(ctx, vdb)
		defer deleteSvcs(ctx, vdb)

		finder := MakeSubclusterFinder(k8sClient, vdb)
		svcs, err := finder.FindServices(ctx, FindInVdb)
		Expect(err).Should(Succeed())
		const SvcsPerSubcluster = 1
		Expect(len(svcs.Items)).Should(Equal(SvcsPerSubcluster))
		svcNames := []string{
			svcs.Items[0].Name,
		}
		Expect(svcNames).Should(ContainElements(
			names.GenExtSvcName(vdb, sc).Name,
		))
	})

	It("should find service objects that do not exist in the vdb", func() {
		vdb := vapi.MakeVDB()
		scNames := []string{"sc1", "sc2"}
		vdb.Spec.Subclusters = []vapi.Subcluster{
			{Name: scNames[0]},
			{Name: scNames[1]},
		}
		sc2 := &vdb.Spec.Subclusters[1]
		createSvcs(ctx, vdb)
		defer deleteSvcs(ctx, vdb)

		// Use a different vdb for the finder so that we can find the service
		// objects missing from it.
		lookupVdb := vapi.MakeVDB()
		lookupVdb.Spec.Subclusters[0] = vapi.Subcluster{Name: scNames[0]}

		finder := MakeSubclusterFinder(k8sClient, lookupVdb)
		svcs, err := finder.FindServices(ctx, FindNotInVdb)
		Expect(err).Should(Succeed())
		const SvcsPerSubcluster = 1
		Expect(len(svcs.Items)).Should(Equal(SvcsPerSubcluster))
		svcNames := []string{
			svcs.Items[0].Name,
		}
		Expect(svcNames).Should(ContainElements(
			names.GenExtSvcName(vdb, sc2).Name,
		))
	})
})
