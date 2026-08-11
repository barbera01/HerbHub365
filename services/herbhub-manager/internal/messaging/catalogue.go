package messaging

import "sort"

type Catalogue struct {
	ID                     string
	Name                   string
	Exchange               ExchangeSpec
	DLX                    ExchangeSpec
	MainQueue              QueueSpec
	DLQ                    QueueSpec
	Bindings               []BindingSpec
	LegacyExpectedBindings []BindingSpec
}

type ExchangeSpec struct {
	Name       string
	Type       string
	Durable    bool
	AutoDelete bool
	Internal   bool
}

type QueueSpec struct {
	Name       string
	Durable    bool
	AutoDelete bool
	Exclusive  bool
	Arguments  map[string]any
}

type BindingSpec struct {
	Exchange   string
	Queue      string
	RoutingKey string
	Arguments  map[string]any
}

func Catalogues() map[string]Catalogue {
	return map[string]Catalogue{
		"watering": {
			ID:   "watering",
			Name: "Watering",
			Exchange: ExchangeSpec{
				Name:       "herbhub.watering",
				Type:       "topic",
				Durable:    true,
				AutoDelete: false,
				Internal:   false,
			},
			DLX: ExchangeSpec{
				Name:       "herbhub.watering.dlx",
				Type:       "topic",
				Durable:    true,
				AutoDelete: false,
				Internal:   false,
			},
			MainQueue: QueueSpec{
				Name:       "watering.queue",
				Durable:    true,
				AutoDelete: false,
				Exclusive:  false,
				Arguments: map[string]any{
					"x-dead-letter-exchange": "herbhub.watering.dlx",
					"x-message-ttl":          float64(86400000),
				},
			},
			DLQ: QueueSpec{
				Name:       "watering.queue.dlq",
				Durable:    true,
				AutoDelete: false,
				Exclusive:  false,
				Arguments:  map[string]any{},
			},
			Bindings: []BindingSpec{
				{Exchange: "herbhub.watering", Queue: "watering.queue", RoutingKey: "watering.#", Arguments: map[string]any{}},
				{Exchange: "herbhub.watering.dlx", Queue: "watering.queue.dlq", RoutingKey: "#", Arguments: map[string]any{}},
			},
			LegacyExpectedBindings: []BindingSpec{
				{Exchange: "herbhub.watering", Queue: "watering.queue", RoutingKey: "watering.basil", Arguments: map[string]any{}},
				{Exchange: "herbhub.watering", Queue: "watering.queue", RoutingKey: "watering.chilli", Arguments: map[string]any{}},
				{Exchange: "herbhub.watering", Queue: "watering.queue", RoutingKey: "watering.oregano", Arguments: map[string]any{}},
				{Exchange: "herbhub.watering", Queue: "watering.queue", RoutingKey: "watering.action", Arguments: map[string]any{}},
			},
		},
		"plant-health": {
			ID:   "plant-health",
			Name: "Plant Health",
			Exchange: ExchangeSpec{
				Name:       "herbhub.plant",
				Type:       "topic",
				Durable:    true,
				AutoDelete: false,
				Internal:   false,
			},
			DLX: ExchangeSpec{
				Name:       "herbhub.plant.dlx",
				Type:       "topic",
				Durable:    true,
				AutoDelete: false,
				Internal:   false,
			},
			MainQueue: QueueSpec{
				Name:       "plant.health.analysis",
				Durable:    true,
				AutoDelete: false,
				Exclusive:  false,
				Arguments: map[string]any{
					"x-dead-letter-exchange": "herbhub.plant.dlx",
					"x-message-ttl":          float64(86400000),
				},
			},
			DLQ: QueueSpec{
				Name:       "plant.health.analysis.dlq",
				Durable:    true,
				AutoDelete: false,
				Exclusive:  false,
				Arguments:  map[string]any{},
			},
			Bindings: []BindingSpec{
				{Exchange: "herbhub.plant", Queue: "plant.health.analysis", RoutingKey: "plant.health.#", Arguments: map[string]any{}},
				{Exchange: "herbhub.plant.dlx", Queue: "plant.health.analysis.dlq", RoutingKey: "#", Arguments: map[string]any{}},
			},
			LegacyExpectedBindings: []BindingSpec{
				{Exchange: "herbhub.plant", Queue: "plant.health.analysis", RoutingKey: "plant.health.left", Arguments: map[string]any{}},
				{Exchange: "herbhub.plant", Queue: "plant.health.analysis", RoutingKey: "plant.health.middle", Arguments: map[string]any{}},
				{Exchange: "herbhub.plant", Queue: "plant.health.analysis", RoutingKey: "plant.health.right", Arguments: map[string]any{}},
			},
		},
	}
}

func SortedCatalogueIDs() []string {
	ids := make([]string, 0, len(Catalogues()))
	for id := range Catalogues() {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}
