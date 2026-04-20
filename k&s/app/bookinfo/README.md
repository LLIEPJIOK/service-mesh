# BookInfo приложение

## Описание

BookInfo - эталонное demo-приложение (источник: [Istio samples](https://github.com/istio/istio/tree/master/samples/bookinfo)), которое используется для проверки базовой работоспособности service mesh.

## Scope MVP

- Подтверждение доступности пользовательской страницы.
- Подтверждение маршрутизации вызовов между сервисами BookInfo.
- Подтверждение распределения трафика на сервис `reviews`.
- Подтверждение наличия метрик в Prometheus/Grafana для BookInfo workload.

## Нормативные требования

1. BookInfo MUST использоваться как основной smoke-workload для демонстрации mesh.
2. Деплой BookInfo MUST выполняться в namespace с включенной sidecar-инъекцией.
3. Повторные запросы к BookInfo MUST показывать распределение трафика на `reviews` (между доступными версиями сервиса).
4. Метрики запросов BookInfo SHOULD быть видны в Prometheus и Grafana.

## Acceptance criteria

| Сценарий      | Критерий успеха                                            |
| ------------- | ---------------------------------------------------------- |
| Доступность   | `productpage` открывается и возвращает корректный ответ    |
| Распределение | В ответах наблюдается обработка `reviews` разными версиями |
| Наблюдаемость | В Prometheus/Grafana есть данные по запросам BookInfo      |

## Failure behavior (summary)

| Ситуация                                 | Поведение MVP                      |
| ---------------------------------------- | ---------------------------------- |
| `productpage` недоступен                 | Демонстрация считается проваленной |
| Нет признаков распределения по `reviews` | Проверка балансировки не пройдена  |
| Метрики отсутствуют в мониторинге        | Проверка наблюдаемости не пройдена |

## Non-goals (MVP)

- Бенчмарк производительности относительно Istio.
- Полная функциональная проверка всех edge-case сценариев приложения.

## Манифесты и запуск в minikube

Готовый набор манифестов расположен в каталоге `manifests/`.

Применение:

```bash
kubectl apply -k k\&s/app/bookinfo/manifests
```

Проверка rollout:

```bash
kubectl rollout status deployment/productpage-v1 -n bookinfo --timeout=10s
kubectl rollout status deployment/details-v1 -n bookinfo --timeout=10s
kubectl rollout status deployment/ratings-v1 -n bookinfo --timeout=10s
kubectl rollout status deployment/reviews-v1 -n bookinfo --timeout=10s
kubectl rollout status deployment/reviews-v2 -n bookinfo --timeout=10s
kubectl rollout status deployment/reviews-v3 -n bookinfo --timeout=10s
```

Открытие страницы в браузере:

```bash
minikube addons enable ingress
kubectl rollout status deployment/ingress-nginx-controller -n ingress-nginx --timeout=120s
minikube tunnel
```

В отдельном терминале:

```bash
echo "http://127.0.0.1/productpage"
```

Ожидаемый путь: `/productpage`.

Fallback через NodePort (если ingress временно недоступен):

```bash
echo "http://$(minikube ip):31380/productpage"
```

## Проверка распределения по reviews

Сгенерируйте серию запросов:

```bash
for i in $(seq 1 20); do
	curl -s "http://127.0.0.1/productpage" | grep -E "(Reviews served by|glyphicon-star|color=black)" | head -n 1
done
```

В серии ответов должны появляться признаки `reviews-v1`, `reviews-v2`, `reviews-v3`.

> [!IMPORTANT]
> Нужно показать работоспособность распределения трафика на сервисы ревью

## Связанные разделы

- [Smoke-тесты](../../test/README.md)
- [Демонстрация service mesh](../../README.md)
- [Балансировка нагрузки sidecar](../../mesh/sidecar/docs/balancing.md)
- [Наблюдаемость sidecar](../../mesh/sidecar/docs/observability.md)
- [Service mesh hook](../../mesh/hook/README.md)
