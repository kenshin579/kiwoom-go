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
