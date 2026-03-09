package crypto

type Algorithm interface {
	Prefix() byte
	FamilySeedPrefix() byte
}
