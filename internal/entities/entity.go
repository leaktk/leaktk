package entities

// Start standardizing the concept of an "entity" in an ECS sense
type Entity int

func (e Entity) ID() int {
	return int(e)
}
