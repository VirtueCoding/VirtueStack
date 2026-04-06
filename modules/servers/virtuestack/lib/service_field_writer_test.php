<?php

declare(strict_types=1);

namespace WHMCS\Database {
    final class Capsule
    {
        /** @var array<string, list<array<string, mixed>>> */
        public static array $tables = [
            'tblcustomfields' => [],
            'tblcustomfieldsvalues' => [],
        ];

        public static function reset(): void
        {
            self::$tables = [
                'tblcustomfields' => [],
                'tblcustomfieldsvalues' => [],
            ];
        }

        public static function table(string $name): FakeQueryBuilder
        {
            return new FakeQueryBuilder($name);
        }
    }

    final class FakeQueryBuilder
    {
        /** @var array<string, mixed> */
        private array $conditions = [];

        public function __construct(private readonly string $table)
        {
        }

        public function where(string $column, mixed $value): self
        {
            $clone = clone $this;
            $clone->conditions[$column] = $value;
            return $clone;
        }

        public function value(string $column): mixed
        {
            foreach (Capsule::$tables[$this->table] as $row) {
                if ($this->matches($row)) {
                    return $row[$column] ?? null;
                }
            }

            return null;
        }

        public function exists(): bool
        {
            foreach (Capsule::$tables[$this->table] as $row) {
                if ($this->matches($row)) {
                    return true;
                }
            }

            return false;
        }

        public function update(array $values): int
        {
            $updated = 0;

            foreach (Capsule::$tables[$this->table] as &$row) {
                if (!$this->matches($row)) {
                    continue;
                }

                foreach ($values as $column => $value) {
                    $row[$column] = $value;
                }

                $updated++;
            }
            unset($row);

            return $updated;
        }

        public function insert(array $values): bool
        {
            Capsule::$tables[$this->table][] = $values;
            return true;
        }

        public function insertGetId(array $values): int
        {
            $nextID = count(Capsule::$tables[$this->table]) + 1;
            $values['id'] = $nextID;
            Capsule::$tables[$this->table][] = $values;

            return $nextID;
        }

        private function matches(array $row): bool
        {
            foreach ($this->conditions as $column => $value) {
                if (($row[$column] ?? null) !== $value) {
                    return false;
                }
            }

            return true;
        }
    }
}

namespace {
    use WHMCS\Database\Capsule;

    /** @var list<string> */
    $activityLog = [];

    function logActivity(string $message): void
    {
        global $activityLog;
        $activityLog[] = $message;
    }

    /**
     * @param array<string, mixed> $conditions
     */
    function get_query_val(string $table, string $column, array $conditions): mixed
    {
        foreach (Capsule::$tables[$table] as $row) {
            if (rowMatches($row, $conditions)) {
                return $row[$column] ?? null;
            }
        }

        return null;
    }

    /**
     * @param array<string, mixed> $values
     * @param array<string, mixed> $conditions
     */
    function update_query(string $table, array $values, array $conditions): void
    {
        foreach (Capsule::$tables[$table] as &$row) {
            if (!rowMatches($row, $conditions)) {
                continue;
            }

            foreach ($values as $column => $value) {
                $row[$column] = $value;
            }
        }
        unset($row);
    }

    /**
     * @param array<string, mixed> $values
     */
    function insert_query(string $table, array $values): int
    {
        $values['id'] = $values['id'] ?? count(Capsule::$tables[$table]) + 1;
        Capsule::$tables[$table][] = $values;

        return (int) $values['id'];
    }

    /**
     * @param array<string, mixed> $row
     * @param array<string, mixed> $conditions
     */
    function rowMatches(array $row, array $conditions): bool
    {
        foreach ($conditions as $column => $value) {
            if (($row[$column] ?? null) !== $value) {
                return false;
            }
        }

        return true;
    }

    require_once __DIR__ . '/../virtuestack.php';

    /**
     * @param mixed $expected
     * @param mixed $actual
     */
    function assertSameValue(string $name, mixed $expected, mixed $actual): void
    {
        if ($expected === $actual) {
            return;
        }

        fwrite(STDERR, $name . PHP_EOL);
        fwrite(STDERR, 'Expected: ' . var_export($expected, true) . PHP_EOL);
        fwrite(STDERR, 'Actual:   ' . var_export($actual, true) . PHP_EOL);
        exit(1);
    }

    function assertContainsText(string $name, string $needle, string $haystack): void
    {
        if (str_contains($haystack, $needle)) {
            return;
        }

        fwrite(STDERR, $name . PHP_EOL);
        fwrite(STDERR, 'Expected to find: ' . $needle . PHP_EOL);
        fwrite(STDERR, 'Actual:           ' . $haystack . PHP_EOL);
        exit(1);
    }

    $validTaskID = '123e4567-e89b-12d3-a456-426614174000';

    Capsule::reset();
    Capsule::$tables['tblcustomfields'][] = [
        'id' => 1,
        'fieldname' => 'task_id',
        'type' => 'product',
    ];

    virtuestack_updateServiceField(101, 'task_id', $validTaskID);

    assertSameValue(
        'legacy wrapper inserts missing task_id rows via validated writer',
        [
            [
                'fieldid' => 1,
                'relid' => 101,
                'value' => $validTaskID,
            ],
        ],
        Capsule::$tables['tblcustomfieldsvalues']
    );

    Capsule::reset();
    $activityLog = [];
    Capsule::$tables['tblcustomfields'][] = [
        'id' => 2,
        'fieldname' => 'provisioning_status',
        'type' => 'product',
    ];
    Capsule::$tables['tblcustomfieldsvalues'][] = [
        'fieldid' => 2,
        'relid' => 202,
        'value' => 'pending',
    ];

    virtuestack_updateServiceField(202, 'provisioning_status', 'not-a-real-status');

    assertSameValue(
        'legacy wrapper rejects invalid provisioning statuses instead of storing drift',
        'pending',
        Capsule::$tables['tblcustomfieldsvalues'][0]['value']
    );
    assertContainsText(
        'invalid provisioning status rejection is logged',
        'Rejected invalid value for field provisioning_status',
        $activityLog[0] ?? ''
    );

    echo "ok\n";
}
