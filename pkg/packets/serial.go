package packets

import "strconv"

type Serial uint32

func (s Serial) String() string {
	return strconv.Itoa(int(s))
}
