package evaluator_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/bradleyjkemp/sigma-go"
	"github.com/bradleyjkemp/sigma-go/evaluator"
)

const testRule = `
id: TEST_RULE
detection:
  a:
    Foo|contains: bar
  b:
    Bar|endswith: baz
  condition: a and b
`

const testConfig = `
title: Test
logsources:
    test:
        product: test

fieldmappings:
    Foo: $.foo
    Bar: $.foobar.baz
`

const testEvent = `
{
	"foo": "foobarbaz",
	"foobar": {
		"baz": "baz"
	},

	"comment": "// random JSON from json-generator.com for more realistic payload size",
    "_id": "60d0b4610f3d918f1790f96a",
    "index": 0,
    "guid": "7d9e0be8-a58c-4295-9716-95828f02c464",
    "isActive": true,
    "balance": "$2,710.89",
    "picture": "http://placehold.it/32x32",
    "age": 40,
    "eyeColor": "blue",
    "name": "Althea Gonzalez",
    "gender": "female",
    "company": "EXOTERIC",
    "email": "altheagonzalez@exoteric.com",
    "phone": "+1 (890) 600-3120",
    "address": "482 Highland Avenue, Garfield, Northern Mariana Islands, 2968",
    "about": "Aliqua culpa proident deserunt dolor sint non. Ea exercitation duis eu elit. Laborum exercitation reprehenderit velit eu eu occaecat duis. Id qui veniam ea sint fugiat do occaecat ut duis laboris.\r\n",
    "registered": "2019-07-16T01:45:06 -01:00",
    "latitude": -8.51841,
    "longitude": 133.547791,
    "tags": [
      "quis",
      "duis",
      "in",
      "fugiat",
      "laborum",
      "incididunt",
      "elit"
    ],
    "friends": [
      {
        "id": 0,
        "name": "Ursula Velez"
      },
      {
        "id": 1,
        "name": "Cecelia Alvarado"
      },
      {
        "id": 2,
        "name": "Mooney Mullen"
      }
    ],
    "greeting": "Hello, Althea Gonzalez! You have 7 unread messages.",
    "favoriteFruit": "apple"
}
`

func BenchmarkRuleEvaluator_Matches(b *testing.B) {
	rule, err := sigma.ParseRule([]byte(testRule))
	if err != nil {
		b.Fatal(err)
	}
	config, err := sigma.ParseConfig([]byte(testConfig))
	if err != nil {
		b.Fatal(err)
	}

	r := evaluator.ForRule(rule, evaluator.WithConfig(config))
	ctx := context.Background()

	b.Run("DecodeAndMatch", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			var event map[string]interface{}
			if err := json.Unmarshal([]byte(testEvent), &event); err != nil {
				b.Fatal(err)
			}
			result, err := r.Matches(ctx, event)
			if err != nil {
				b.Fatal(err)
			}
			if !result.Match {
				b.Fatal("event should have matched")
			}
		}
	})
	b.Run("JustMatch", func(b *testing.B) {
		var event map[string]interface{}
		if err := json.Unmarshal([]byte(testEvent), &event); err != nil {
			b.Fatal(err)
		}
		for i := 0; i < b.N; i++ {
			result, err := r.Matches(ctx, event)
			if err != nil {
				b.Fatal(err)
			}
			if !result.Match {
				b.Fatal("event should have matched")
			}
		}
	})
}
