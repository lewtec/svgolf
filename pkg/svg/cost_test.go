package svg

import "testing"

func TestCost(t *testing.T) {
	t.Parallel()
	if got := Cost(Node{}); got != 0 {
		t.Errorf("KindInvalid Cost = %d; want 0", got)
	}
	if got := Cost(NewGroup().Node()); got != 0 {
		t.Errorf("empty group Cost = %d; want 0", got)
	}
	if got := Cost(NewCircle().Node()); got != 1 {
		t.Errorf("circle Cost = %d; want 1", got)
	}
	if got := Cost(NewEllipse().Node()); got != 2 {
		t.Errorf("ellipse Cost = %d; want 2", got)
	}
	if got := Cost(NewRect().WithWidth(10).WithHeight(10).Node()); got != 1 {
		t.Errorf("axis-aligned rect Cost = %d; want 1", got)
	}
	if got := Cost(NewRect().WithWidth(10).WithHeight(10).WithRX(2).Node()); got != 2 {
		t.Errorf("rounded rect Cost = %d; want 2", got)
	}
	// rx stored 100, clamped to width/2 = 5; still rounded
	if got := Cost(NewRect().WithWidth(10).WithHeight(10).WithRX(100).Node()); got != 2 {
		t.Errorf("oversize rx Cost = %d; want 2", got)
	}
	// rx clamped to 0 when width is 0
	if got := Cost(NewRect().WithRX(4).Node()); got != 1 {
		t.Errorf("zero-size rect with rx Cost = %d; want 1 (clamped)", got)
	}
	poly, err := NewPolygon().WithPoints([][2]float64{{0, 0}, {1, 0}, {0, 1}})
	if err != nil {
		t.Fatal(err)
	}
	if got := Cost(poly.Node()); got != 4 {
		t.Errorf("polygon Cost = %d; want 4", got)
	}
	g := NewGroup().Append(NewCircle().Node(), NewEllipse().Node())
	if got := Cost(g.Node()); got != 3 {
		t.Errorf("group Cost = %d; want 3", got)
	}
	doc := NewDocument(10, 10).Append(NewCircle().Node(), NewRect().WithWidth(4).WithHeight(4).Node())
	if got := CostDocument(doc); got != 2 {
		t.Errorf("CostDocument = %d; want 2", got)
	}
}
