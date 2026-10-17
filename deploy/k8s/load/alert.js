// Ровная нагрузка на 4 минуты: за это время ломаем базу и смотрим,
// как алерт проходит inactive -> pending -> firing -> inactive.
// Порогов нет: ошибки здесь ожидаемы.
import http from 'k6/http';

export const options = {
  scenarios: {
    steady: {
      executor: 'constant-arrival-rate',
      rate: 30,
      timeUnit: '1s',
      duration: '4m',
      preAllocatedVUs: 20,
      maxVUs: 100,
    },
  },
};

export default function () {
  http.get('http://order-service:8083/orders?limit=10', { timeout: '5s' });
}
