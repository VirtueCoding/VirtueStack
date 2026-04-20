/**
 * VirtueStack k6 customer VM list load test.
 *
 * Exercises GET /api/v1/customer/vms with pagination and optional status/search filters.
 *
 * Usage:
 *   k6 run tests/load/k6-customer-list.js
 *
 * Required environment variables:
 *   CUSTOMER_TOKEN
 *
 * Optional environment variables:
 *   BASE_URL
 */

import http from 'k6/http';
import { check, sleep } from 'k6';
import { Counter, Rate, Trend } from 'k6/metrics';
import { randomIntBetween } from 'https://jslib.k6.io/k6-utils/1.2.0/index.js';

const BASE_URL = __ENV.BASE_URL || 'http://localhost:8080';
const CUSTOMER_TOKEN = __ENV.CUSTOMER_TOKEN || '';
const statuses = ['running', 'stopped', 'suspended', 'error', 'provisioning'];
const searches = ['', 'web', 'db', 'prod', 'dev'];

const listDuration = new Trend('customer_list_duration');
const listErrors = new Rate('customer_list_errors');
const listRequests = new Counter('customer_list_requests');

export const options = {
	stages: [
		{ duration: '30s', target: 20 },
		{ duration: '60s', target: 50 },
		{ duration: '60s', target: 100 },
		{ duration: '30s', target: 0 },
	],
	thresholds: {
		http_req_duration: ['p(95)<500'],
		http_req_failed: ['rate<0.01'],
		customer_list_duration: ['p(95)<500'],
		customer_list_errors: ['rate<0.01'],
	},
	tags: {
		test: 'customer-list-load-test',
		environment: __ENV.K6_ENV || 'development',
	},
};

function headers() {
	return {
		'Content-Type': 'application/json',
		Authorization: `Bearer ${CUSTOMER_TOKEN}`,
	};
}

function buildQuery() {
	const page = randomIntBetween(1, 20);
	const perPage = [10, 20, 50][randomIntBetween(0, 2)];
	const status = statuses[randomIntBetween(0, statuses.length - 1)];
	const search = searches[randomIntBetween(0, searches.length - 1)];
	const params = new URLSearchParams({ page: String(page), per_page: String(perPage), status });
	if (search) {
		params.set('search', search);
	}
	return params.toString();
}

export default function () {
	const response = http.get(
		`${BASE_URL}/api/v1/customer/vms?${buildQuery()}`,
		{ headers: headers(), tags: { operation: 'customer_list_vms' } },
	);
	listRequests.add(1);
	listDuration.add(response.timings.duration);

	const success = check(response, {
		'customer list: status is 200': (r) => r.status === 200,
		'customer list: has data and meta': (r) => {
			try {
				const body = JSON.parse(r.body);
				return body && body.data !== undefined && body.meta !== undefined;
			} catch {
				return false;
			}
		},
		'customer list: response time < 500ms': (r) => r.timings.duration < 500,
	});
	listErrors.add(!success);

	sleep(randomIntBetween(1, 2));
}

export function setup() {
	if (!CUSTOMER_TOKEN) {
		throw new Error('CUSTOMER_TOKEN is required');
	}
}
