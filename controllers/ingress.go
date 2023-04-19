package controllers

import (
	"context"
	v1 "github.com/fhivemind/plant-operator/api/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"reflect"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const ingressCondition v1.ConditionType = "deployment-ingress"

// TODO: this will not work for ACME challenge, fix it!
func (r *PlantReconciler) manageIngress(ctx context.Context, plant *v1.Plant, tlsSecretName string) (*networkingv1.Ingress, error) {
	logger := log.FromContext(ctx)

	// Create ingress if not found
	ingress := &networkingv1.Ingress{}
	err := r.Client.Get(ctx, types.NamespacedName{Name: plant.Name, Namespace: plant.Namespace}, ingress)
	if err != nil {
		if errors.IsNotFound(err) {
			if err := r.Client.Create(ctx, ingress); err != nil { // create if not found
				return nil, err
			}
			if err := controllerutil.SetControllerReference(plant, ingress, r.Scheme); err != nil { // set ownership
				return nil, err
			}
			logger.Info("successfully created ingress")
		} else {
			logger.Info("failed to create ingress")
			return nil, err
		}
	}

	// Update ingress if required
	requiredIngress := defineIngress(plant, tlsSecretName)
	if !reflect.DeepEqual(requiredIngress.Spec, ingress.Spec) {
		ingress.ObjectMeta = requiredIngress.ObjectMeta
		err = r.Client.Update(ctx, ingress)
		if err != nil {
			return nil, err
		}
		logger.Info("successfully updated ingress")
	}

	// TODO: handle resource changes by using watchers to handle Plant status updates
	plant.UpdateCondition(ingressCondition, metav1.ConditionTrue)

	// Return back
	return ingress, nil
}

func defineIngress(plant *v1.Plant, tlsSecretName string) *networkingv1.Ingress {
	var tlsIngress []networkingv1.IngressTLS
	if tlsSecretName != "" {
		tlsIngress = []networkingv1.IngressTLS{
			{
				Hosts: []string{
					plant.Spec.Host,
				},
				SecretName: tlsSecretName,
			},
		}
	}
	pathType := networkingv1.PathTypePrefix
	return &networkingv1.Ingress{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Ingress",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      plant.Name,
			Namespace: plant.Namespace,
			Labels: map[string]string{ // TODO: fill better
				"app": plant.Name,
			},
		},
		Spec: networkingv1.IngressSpec{
			IngressClassName: plant.Spec.IngressClassName,
			TLS:              tlsIngress,
			Rules: []networkingv1.IngressRule{
				{
					Host: plant.Spec.Host,
					IngressRuleValue: networkingv1.IngressRuleValue{
						HTTP: &networkingv1.HTTPIngressRuleValue{
							Paths: []networkingv1.HTTPIngressPath{
								{
									Path:     "/",
									PathType: &pathType,
									Backend: networkingv1.IngressBackend{
										Service: &networkingv1.IngressServiceBackend{ // TODO: handle better
											Name: plant.Name,
											Port: networkingv1.ServiceBackendPort{
												Name: "http-service-port",
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
}