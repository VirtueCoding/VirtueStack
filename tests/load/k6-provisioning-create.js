/**
 * VirtueStack k6 provisioning create load test.
 *
 * Exercises POST /api/v1/provisioning/vms with 10, 50, and 100 concurrent VUs.
 *
 * Usage:
 *   k6 run tests/load/k6-provisioning-create.js
 *
 * Required environment variables:
 *   PROVISIONING_API_KEY
 *
 * Optional environment variables:
 *   BASE_URL
 *   CUSTOMER_ID
 *   PLAN_ID
 *   TEMPLATE_ID
 *   LOCATION_ID
 */

import http from 'k6/http';
import { check, sleep } from 'k6';
import { Rate, Trend, Counter } from 'k6/metrics';
import { randomString, randomIntBetween } from 'https://jslib.k6.io/k6-utils/1.2.0/index.js';

const BASE_URL = __ENV.BASE_URL || 'http://localhost:8080';
const PROVISIONING_API_KEY = __ENV.PROVISIONING_API_KEY || '';
const CUSTOMER_ID = __ENV.CUSTOMER_ID || '00000000-0000-0000-0000-000000000001';
const PLAN_ID = __ENV.PLAN_ID || '00000000-0000-0000-0000-000000000001';
const TEMPLATE_ID = __ENV.TEMPLATE_ID || '00000000-0000-0000-0000-000000000001';
const LOCATION_ID = __ENV.LOCATION_ID || '';

const createDuration = new Trend('provisioning_create_duration');
const createErrors = new Rate('provisioning_create_errors');
const createRequests = new Counter('provisioning_create_requests');

export const options = {
	scenarios: {
		provisioning_create_10: {
			executor: 'constant-vus',
			vus: 10,
			duration: '90s',
		},
		provisioning_create_50: {
			executor: 'constant-vus',
			startTime: '2m',
			vus: 50,
			duration: '90s',
		},
		provisioning_create_100: {
			executor: 'constant-vus',
			startTime: '4m',
			vus: 100,
			duration: '90s',
		},
	},
	thresholds: {
		http_req_duration: ['p(95)<500'],
		http_req_failed: ['rate<0.01'],
		provisioning_create_duration: ['p(95)<1200'],
		provisioning_create_errors: ['rate<0.01'],
	},
	tags: {
		test: 'provisioning-create-load-test',
		environment: __ENV.K6_ENV || 'development',
	},
};

function provisioningHeaders() {
	return {
		'Content-Type': 'application/json',
		'X-API-Key': PROVISIONING_API_KEY,
		'Idempotency-Key': `${__VU}-${__ITER}-${randomString(12)}`,
	};
}

function createPayload() {
	const payload = {
		customer_id: CUSTOMER_ID,
		plan_id: PLAN_ID,
		template_id: TEMPLATE_ID,
		hostname: `k6-provision-${randomString(8)}`.toLowerCase(),
		ssh_keys: [],
		whmcs_service_id: (__VU * 1000000) + __ITER,
	};
	if (LOCATION_ID) {
		payload.location_id = LOCATION_ID;
	}
	return JSON.stringify(payload);
}

export default function () {
	const response = http.post(
		`${BASE_URL}/api/v1/provisioning/vms`,
		createPayload(),
		{ headers: provisioningHeaders(), tags: { operation: 'provisioning_create' } },
	);
	createRequests.add(1);
	createDuration.add(response.timings.duration);

	const success = check(response, {
		'provisioning create: status is 202 or known validation/conflict': (r) =>
			r.status === 202 || r.status === 400 || r.status === 409,
		'provisioning create: response time < 1500ms': (r) => r.timings.duration < 1500,
	});
	createErrors.add(!success);

	sleep(randomIntBetween(1, 3));
}

export function setup() {
	if (!PROVISIONING_API_KEY) {
		throw new Error('PROVISIONING_API_KEY is required');
	}
}
