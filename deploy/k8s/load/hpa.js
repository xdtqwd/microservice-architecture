// Нагрузка для HPA: разгон до 400 rps, две минуты на пике, остановка.
import http from 'k6/http';

export const options = {
  scenarios: {
    ramp: {
      executor: 'ramping-arrival-rate',
      startRate: 50, timeUnit: '1s',
      preAllocatedVUs: 50, maxVUs: 300,
      stages: [
        { target: 400, duration: '30s' },
        { target: 400, duration: '2m' },
        { target: 0, duration: '10s' },
      ],
    },
  },
};

export default function () {
  http.get('http://order-service:8083/orders?limit=50');
}
