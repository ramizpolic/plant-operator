package workflow

import (
	"context"
	"fmt"
	certv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	cmmeta "github.com/cert-manager/cert-manager/pkg/apis/meta/v1"
	apiv1 "github.com/fhivemind/plant-operator/api/v1"
	"github.com/fhivemind/plant-operator/pkg/resource"
	"github.com/fhivemind/plant-operator/pkg/utils"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// newTlsOrNopHandler creates either a resource.Executor or resource.NopExecutor depending on the state of Plant.
// Following cases can occur:
//
//		 a) No Tls requested, returns nil and resource.NopExecutor
//		 b) Tls Secret requested, returns plant.Spec.TlsSecretName and resource.NopExecutor
//	 	 c) CertIssuer requested, returns secret name issued by CertIssuer and certificate handler
//
// The workflow selection is handled from Plant resource.
func (m *manager) newTlsOrNopHandler(plant *apiv1.Plant) (*string, resource.Executor[*certv1.Certificate]) {
	// a) Nothing selected, return nil and nop handler
	if plant.Spec.TlsSecretName == nil && plant.Spec.TlsCertIssuerRef == nil {
		return nil, resource.NopExecutor[*certv1.Certificate]("Certificate")
	}

	// b) Tls only, return the secret name and nop handler
	if plant.Spec.TlsSecretName != nil {
		return plant.Spec.TlsSecretName, resource.NopExecutor[*certv1.Certificate]("Certificate")
	}

	// c) CertManager only, return certificate secret name and handler
	// Create expected object
	expected := defineCertificate(plant)
	m.Client().Scheme().Default(expected)

	// Return handler
	return &expected.Spec.SecretName, resource.Executor[*certv1.Certificate]{
		Name: "Certificate",
		FetchFunc: func(ctx context.Context, object *certv1.Certificate) error {
			return m.Client().Get(ctx, types.NamespacedName{Namespace: expected.Namespace, Name: expected.Name}, object)
		},
		CreateFunc: func(ctx context.Context, object *certv1.Certificate) error {
			expected.DeepCopyInto(object) // fill with required values
			if err := controllerutil.SetControllerReference(plant, object, m.Client().Scheme()); err != nil {
				return err
			}
			return m.Client().Create(ctx, object)
		},
		UpdateFunc: func(ctx context.Context, object *certv1.Certificate) (bool, error) {
			diff := utils.Diff(&expected.Spec, &object.Spec)
			if diff.NotEqual() {
				expected.Spec.DeepCopyInto(&object.Spec)
				utils.MergeMapsSrcDst(expected.Labels, object.Labels)
				return true, m.Client().Update(ctx, object)
			}
			return false, diff.Error()
		},
		IsReady: func(_ context.Context, object *certv1.Certificate) bool {
			for _, cond := range object.Status.Conditions {
				if cond.Type == certv1.CertificateConditionReady &&
					cond.Status == cmmeta.ConditionTrue {
					return true
				}
			}
			return false
		},
	}
}

func defineCertificate(plant *apiv1.Plant) *certv1.Certificate {
	return &certv1.Certificate{
		ObjectMeta: metav1.ObjectMeta{
			Name:      plant.Name,
			Namespace: plant.Namespace,
			Labels:    plant.OperatorLabels(),
		},
		Spec: certv1.CertificateSpec{
			SecretName: fmt.Sprintf("%s-tls", plant.Name),
			DNSNames:   []string{plant.Spec.Host},
			IssuerRef:  *plant.Spec.TlsCertIssuerRef,
		},
	}
}