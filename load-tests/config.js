export const config = {
    baseUrl: __ENV.BASE_URL || 'http://localhost:8083',
    vus: parseInt(__ENV.VUS) || 10,
    duration: __ENV.DURATION || '30s',
};