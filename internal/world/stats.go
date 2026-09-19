package world

type StatName string

const (
	StatHP           StatName = "hp"
	StatMana         StatName = "mana"
	StatAttack       StatName = "attack"
	StatDefense      StatName = "defense"
	StatMagicAttack  StatName = "magic_attack"
	StatMagicDefense StatName = "magic_defense"
	StatSpeed        StatName = "speed"
)

type Stats struct {
	HP           int `json:"hp"`
	Mana         int `json:"mana"`
	Attack       int `json:"attack"`
	Defense      int `json:"defense"`
	MagicAttack  int `json:"magic_attack"`
	MagicDefense int `json:"magic_defense"`
	Speed        int `json:"speed"`
}

func (s Stats) Add(o Stats) Stats {
	return Stats{
		HP:           s.HP + o.HP,
		Mana:         s.Mana + o.Mana,
		Attack:       s.Attack + o.Attack,
		Defense:      s.Defense + o.Defense,
		MagicAttack:  s.MagicAttack + o.MagicAttack,
		MagicDefense: s.MagicDefense + o.MagicDefense,
		Speed:        s.Speed + o.Speed,
	}
}

func (s Stats) Mul(n int) Stats {
	return Stats{
		HP:           s.HP * n,
		Mana:         s.Mana * n,
		Attack:       s.Attack * n,
		Defense:      s.Defense * n,
		MagicAttack:  s.MagicAttack * n,
		MagicDefense: s.MagicDefense * n,
		Speed:        s.Speed * n,
	}
}
