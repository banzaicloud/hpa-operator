### Horizontal Pod Autoscaler operator

You may not want nor can edit a Helm chart just to add an autoscaling feature. Nearly all charts supports **custom annotations** so we believe that it would be a good idea to be able to setup autoscaling just by adding some simple annotations to your deployment. 

We have open sourced a [Horizontal Pod Autoscaler operator](https://github.com/banzaicloud/hpa-operator). This operator watches for your `Deployment` or `StatefulSet` and automatically creates an *HorizontalPodAutoscaler* resource, should you provide the correct autoscale annotations.

- [Horizontal Pod Autoscaler operator](https://github.com/banzaicloud/hpa-operator)
- [Horizontal Pod Autoscaler operator Helm chart](https://github.com/banzaicloud/hpa-operator/tree/master/deploy/charts/hpa-operator)

### Autoscale by annotations

Autoscale annotations can be placed:

- directly on Deployment / StatefulSet:

 ```
  apiVersion: extensions/v1beta1
  kind: Deployment
  metadata:
    name: example
    labels:
    annotations:
      hpa.autoscaling.banzaicloud.io/minReplicas: "1"
      hpa.autoscaling.banzaicloud.io/maxReplicas: "3"
      cpu.hpa.autoscaling.banzaicloud.io/targetAverageUtilization: "70"
  ```

- or on `spec.template.metadata.annotations`:

 ```
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
            hpa.autoscaling.banzaicloud.io/minReplicas: "1"
            hpa.autoscaling.banzaicloud.io/maxReplicas: "3"
            cpu.hpa.autoscaling.banzaicloud.io/targetAverageUtilization: "70"
  ```  

The [Horizontal Pod Autoscaler operator](https://github.com/banzaicloud/hpa-operator) takes care of creating, deleting, updating HPA, with other words keeping in sync with your deployment annotations.

### Annotations explained

All annotations must contain the `autoscaling.banzaicloud.io` prefix. It is required to specify minReplicas/maxReplicas and at least one metric to be used for autoscale. You can add *Resource* type metrics for cpu & memory and *Pods* type metrics.
Let's see what kind of annotations can be used to specify metrics:

- ``cpu.hpa.autoscaling.banzaicloud.io/targetAverageUtilization: "{targetAverageUtilizationPercentage}"`` - adds a Resource type metric for cpu with targetAverageUtilizationPercentage set as specified, where targetAverageUtilizationPercentage should be an int value between [1-100]

- ``cpu.hpa.autoscaling.banzaicloud.io/targetAverageValue: "{targetAverageValue}"`` - adds a Resource type metric for cpu with targetAverageValue set as specified, where targetAverageValue is a [Quantity](https://godoc.org/k8s.io/apimachinery/pkg/api/resource#Quantity).

- ``memory.hpa.autoscaling.banzaicloud.io/targetAverageUtilization: "{targetAverageUtilizationPercentage}"`` - adds a Resource type metric for memory with targetAverageUtilizationPercentage set as specified, where targetAverageUtilizationPercentage should be an int value between [1-100]

- ``memory.hpa.autoscaling.banzaicloud.io/targetAverageValue: "{targetAverageValue}"`` - adds a Resource type metric for memory with targetAverageValue set as specified, where targetAverageValue is a [Quantity](https://godoc.org/k8s.io/apimachinery/pkg/api/resource#Quantity).

- ``pod.hpa.autoscaling.banzaicloud.io/customMetricName: "{targetAverageValue}"`` - adds a Pods type metric with targetAverageValue set as specified, where targetAverageValue is a [Quantity](https://godoc.org/k8s.io/apimachinery/pkg/api/resource#Quantity).

> To use custom metrics from *Prometheus*, you have to deploy `Prometheus Adapter` and `Metrics Server`, explained in detail in our previous post about [using HPA with custom metrics](https://banzaicloud.com/blog/k8s-horizontal-pod-autoscaler/)

#### Custom metrics from version 0.1.5

From version 0.1.5 we have removed support for *Pod* type custom metrics and added support for Prometheus backed custom metrics exposed by [Kube Metrics Adapter](https://github.com/zalando-incubator/kube-metrics-adapter).
To setup HPA based on Prometheus one has to setup the following deployment annotations:

``
prometheus.customMetricName.hpa.autoscaling.banzaicloud.io/query: "sum({kubernetes_pod_name=~"^YOUR_POD_NAME.*",__name__=~"YOUR_PROMETHUES_METRICNAME"})"
prometheus.customMetricName.hpa.autoscaling.banzaicloud.io/targetValue: "{targetValue}"
prometheus.customMetricName.hpa.autoscaling.banzaicloud.io/targetAverageValue: "{targetAverageValue}"
``

The query should be a syntactically correct Prometheus query. Pay attention to select only metrics related to your *Deployment* / *Pod* / *Service*. 
You should specify either targetValue or targetAverageValue, in which case metric value is averaged with current replica count.


### Quick usage example

Let's pick **Kafka** as an example chart, from our curated list of [Banzai Cloud Helm charts](https://github.com/banzaicloud/banzai-charts/tree/master/kafka). The Kafka chart by default doesn't contains any HPA resources, however it allows specifying Pod annotations as params so it's a good example to start with. Now let's see how you can add a simple cpu based autoscale rule for Kafka brokers by adding some simple annotations:

  1. Deploy operator

   ```
    helm install banzaicloud-stable/hpa-operator
   ```

  2. Deploy Kafka chart, with autoscale annotations

   ```
    cat > values.yaml <<EOF
    {
        "statefullset": {
           "annotations": {
                hpa.autoscaling.banzaicloud.io/minReplicas: "3"
                hpa.autoscaling.banzaicloud.io/maxReplicas: "8"
                cpu.hpa.autoscaling.banzaicloud.io/targetAverageUtilization: "60"
           }
        }
    }
    EOF

    helm install -f values.yaml banzaicloud-stable/kafka
   ```

  1. Check if HPA is created

   ```
    kubectl get hpa

    NAME      REFERENCE           TARGETS           MINPODS   MAXPODS   REPLICAS   AGE
    kafka     StatefulSet/kafka   3% / 60%          3         8         1          1m
  ```

Happy **Autoscaling!**
