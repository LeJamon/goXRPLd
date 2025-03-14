package crypto

type KeyType interface {
	Prefix() byte
	FamilySeedPrefix() byte
}
