package plan

// ItemDiscount — суммарная скидка на позицию: каждая взятая покупка из
// Discounts снимает Amount XP. Отложенный урок скидку не даёт: он уедет
// в следующую жизнь и здесь ещё не оплачен.
func (p *Plan) ItemDiscount(it *ShopItem) float64 {
	if it == nil {
		return 0
	}
	var sum float64
	for _, d := range it.Discounts {
		if d.Key == "" || d.Key == it.Key || d.Amount <= 0 {
			continue
		}
		if p.Picked(d.Key) && !p.IsDeferred(d.Key) {
			sum += d.Amount
		}
	}
	return sum
}

// ItemCost — цена позиции с учётом скидок. Дешевле нуля покупка не бывает:
// скидка может закрыть всю цену, но не превратить её в доход.
func (p *Plan) ItemCost(it *ShopItem) float64 {
	if it == nil {
		return 0
	}
	cost := it.Cost - p.ItemDiscount(it)
	if cost < 0 {
		return 0
	}
	return cost
}
