/**
 * VirtueStack k6 power operations load test.
 *
 * Exercises POST /api/v1/provisioning/vms/:id/power for start/stop/restart.
 *
 * Usage:
 *   k6 run tests/load/k6-power-operations.js
 *
 * Required environment variables:
 *   PROVISIONING_API_KEY
 *   TEST_VM_ID
 *
 * Optional environment variables:
 *   BASE_URL
 */

import http from 'k6/http';
import { check, sleep } from 'k6';
import { Counter, Rate, Trend } from 'k6/metrics';
import { randomIntBetween } from 'https://jslib.k6.io/k6-utils/1.2.0/index.js';

const BASE_URL = __ENV.BASE_URL || 'http://localhost:8080';
const PROVISIONING_API_KEY = __ENV.PROVISIONING_API_KEY || '';
const TEST_VM_ID = __ENV.TEST_VM_ID || '';
const operations = ['start', 'stop', 'restart'];

const powerDuration = new Trend('power_operation_duration');
const powerErrors = new Rate('power_operation_errors');
const powerRequests = new Counter('power_operation_requests');

export const options = {
	stages: [
		{ duration: '30s', target: 20 },
		{ duration: '60s', target: 50 },
		{ duration: '90s', target: 100 },
		{ duration: '30s', target: 0 },
	],
	thresholds: {
		http_req_duration: ['p(95)<500'],
		http_req_failed: ['rate<0.01'],
		power_operation_duration: ['p(95)<1000'],
		power_operation_errors: ['rate<0.01'],
	},
	tags: {
		test: 'power-operations-load-test',
		environment: __ENV.K6_ENV || 'development',
	},
};

function headers() {
	return {
		'Content-Type': 'application/json',
		'X-API-Key': PROVISIONING_API_KEY,
	};
}

function operationPayload() {
	const operation = operations[randomIntBetween(0, operations.length - 1)];
	return JSON.stringify({ operation });
}

export default function () {
	const response = http.post(
		`${BASE_URL}/api/v1/provisioning/vms/${TEST_VM_ID}/power`,
		operationPayload(),
		{ headers: headers(), tags: { operation: 'provisioning_power_operation' } },
	);
	powerRequests.add(1);
	powerDuration.add(response.timings.duration);

	const success = check(response, {
		'power operation: status is 200 or known conflict': (r) =>
			r.status === 200 || r.status === 400 || r.status === 409,
		'power operation: response time < 1200ms': (r) => r.timings.duration < 1200,
	});
	powerErrors.add(!success);

	sleep(randomIntBetween(1, 2));
}

export function setup() {
	if (!PROVISIONING_API_KEY) {
		throw new Error('PROVISIONING_API_KEY is required');
	}
	if (!TEST_VM_ID) {
		throw new Error('TEST_VM_ID is required');
	}
}
