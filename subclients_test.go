package kiwoom_test

import (
	"reflect"
	"testing"

	"github.com/kenshin579/kiwoom-go"
)

// TestNewClient_모든_하위_클라이언트가_배선된다 는 Clients 의 필드가 하나도 비지 않았는지 본다.
//
// reflect 로 훑는 이유: 그룹이 늘 때마다 이 테스트를 고쳐야 한다면, 고치는 것을 깜빡한
// 그룹이 정확히 잡히지 않는다.
func TestNewClient_모든_하위_클라이언트가_배선된다(t *testing.T) {
	c, err := kiwoom.NewClient("AK", "SK", kiwoom.WithMock())
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	v := reflect.ValueOf(c.Clients)
	if v.NumField() == 0 {
		t.Fatal("Clients 에 필드가 없다 — 생성이 빈 것 아닌가")
	}
	for i := 0; i < v.NumField(); i++ {
		if v.Field(i).IsNil() {
			t.Errorf("%s 가 nil 이다", v.Type().Field(i).Name)
		}
	}
}

// TestClients_그룹_개수 는 하위 클라이언트가 조용히 사라지는 것을 잡는다.
//
// 그룹을 의도적으로 더하거나 뺐다면 이 숫자를 함께 고쳐라 — 고치는 행위 자체가
// "정말 뺄 생각이었나" 를 한 번 묻는다.
func TestClients_그룹_개수(t *testing.T) {
	const want = 25
	if got := reflect.TypeOf(kiwoom.Clients{}).NumField(); got != want {
		t.Errorf("하위 클라이언트 %d개, 기대 %d개", got, want)
	}
}

// TestClients_필드가_Client_메서드에_가려지지_않는다 는 루트 Client 에 하위 클라이언트와
// 같은 이름의 메서드가 생기는 것을 잡는다.
//
// **컴파일러가 안 잡아 준다.** Client 가 Clients 를 임베딩하므로 승격된 필드는 깊이 1 에
// 있고, 루트에 func (c *Client) OverseasOrder(...) 를 더하면 깊이 0 인 그 메서드가 조용히
// 이긴다. 이 저장소는 그대로 깨끗하게 컴파일되고, 깨지는 것은 c.OverseasOrder.Xxx() 를
// 쓰고 있던 **사용자의 빌드**다. 그래서 여기서 이름 충돌을 직접 본다.
func TestClients_필드가_Client_메서드에_가려지지_않는다(t *testing.T) {
	// 값 리시버 메서드도 포인터 메서드 집합에 들어오므로 *Client 로 훑는다.
	ct := reflect.TypeOf(&kiwoom.Client{})
	methods := make(map[string]bool, ct.NumMethod())
	for i := 0; i < ct.NumMethod(); i++ {
		methods[ct.Method(i).Name] = true
	}

	ft := reflect.TypeOf(kiwoom.Clients{})
	for i := 0; i < ft.NumField(); i++ {
		name := ft.Field(i).Name
		if methods[name] {
			t.Errorf("Client 에 %s 메서드가 있어 Clients.%s 필드를 가린다 — "+
				"c.%s 가 하위 클라이언트가 아니라 메서드로 풀린다. 둘 중 하나의 이름을 바꿔라", name, name, name)
		}
	}
}
