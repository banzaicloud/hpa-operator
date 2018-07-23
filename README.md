### Horizontal Pod Autoscaler operator

You may not want nor can edit a Helm chart just to add an autoscaling feature. Nearly all charts supports **custom annotations** so we believe that it would be a good idea to be able to setup autoscaling just by adding some simple annotations to your deployment. 

We have open sourced a [Horizontal Pod Autoscaler operator](https://github.com/banzaicloud/hpa-operator). This operator watches for your `Deployment` or `StatefulSet` and automatically creates an *HorizontalPodAutoscaler* resource, should you provide the correct autoscale annotations.

- [Horizontal Pod Autoscaler operator](https://github.com/banzaicloud/hpa-operator)
- [Horizontal Pod Autoscaler operator Helm chart](https://github.com/banzaicloud/banzai-charts/tree/master/hpa-operator)

### Autoscale by annotations

Autoscale annotations can be placed:

- directly on Deployment / StatefulSet:

  {{< highlight yml>}}
  apiVersion: extensions/v1beta1
  kind: Deployment
  metadata:
    name: example
    labels:
    annotations:
      autoscale/minReplicas: "1"
      autoscale/maxReplicas: "3"
      autoscale/cpu: "70"
  {{< / highlight >}}

- or on `spec.template.metadata.annotations`:

  {{< highlight yml>}}
  apiVersion: extensions/v1beta1
  kind: Deployment
  ...
  spec:
    replicas: 3
    template:
      metadata:
        labels:
          ...
        annotations:
          autoscale/minReplicas: "1"
          autoscale/maxReplicas: "3"
          autoscale/cpu: "70"
  {{< / highlight >}}      

The [Horizontal Pod Autoscaler operator](https://github.com/banzaicloud/hpa-operator) takes care of creating, deleting, updating HPA, with other words keeping in sync with your deployment annotations.

### Annotations explained

All annotations must be prefixed with `autoscale`. It is required to specify minReplicas/maxReplicas and at least one metric to be used for autoscale. You can add *Resource* type metrics for cpu & memory and *Pods* type metrics.
Let's see what kind of annotations can be used to specify metrics:

- ``autoscale/cpu: "{targetAverageUtilizationPercentage}"`` - adds a Resource type metric for cpu with targetAverageUtilization set as specified, where targetAverageUtilizationPercentage should be an int value between [1-100]

- ``autoscale/memory: "{targetAverageValue}"`` - adds a Resource type metric for memory with targetAverageValue set as specified, where targetAverageValue is a [Quantity](https://godoc.org/k8s.io/apimachinery/pkg/api/resource#Quantity).

- ``autoscale.pod/custom_metric_name: "{targetAverageValue}"`` - adds a Pods type metric with targetAverageValue set as specified, where targetAverageValue is a [Quantity](https://godoc.org/k8s.io/apimachinery/pkg/api/resource#Quantity).

> To use custom metrics from *Prometheus*, you have to deploy `Prometheus Adapter` and `Metrics Server`, explained in detail in our previous post about [using HPA with custom metrics](https://banzaicloud.com/blog/k8s-horizontal-pod-autoscaler/)

### Quick usage example

Let's pick **Kafka** as an example chart, from our curated list of [Banzai Cloud Helm charts](https://github.com/banzaicloud/banzai-charts/tree/master/kafka). The Kafka chart by default doesn't contains any HPA resources, however it allows specifying Pod annotations as params so it's a good example to start with. Now let's see how you can add a simple cpu based autoscale rule for Kafka brokers by adding some simple annotations:

  1. Deploy operator

    {{< highlight shell>}}
    helm install banzaicloud-stable/hpa-operator
    {{< / highlight >}}

  1. Deploy Kafka chart, with autoscale annotations

    {{< highlight shell>}}
    cat > values.yaml <<EOF
    {
        "statefullset": {
           "annotations": {
               "autoscale/minReplicas": "3",
               "autoscale/maxReplicas": "8",
               "autoscale/cpu": "60"
           }
        }
    }
    EOF

    helm install -f values.yaml banzaicloud-stable/kafka
    {{< / highlight >}}

  1. Check if HPA is created

    {{< highlight shell>}}
    kubectl get hpa

    NAME      REFERENCE           TARGETS           MINPODS   MAXPODS   REPLICAS   AGE
    kafka     StatefulSet/kafka   3% / 60%          3         8         1          1m
    {{< / highlight >}}

Happy **Autoscaling!**
