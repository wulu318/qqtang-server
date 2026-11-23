package shopcatalog

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
)

// PriceTable keeps shop pricing out of code so operators can retune the
// economy without rebuilding. The server and the Commodity.ini generator must
// load the same file, otherwise startup validation rejects the generated file.
type PriceTable struct {
	Default     uint32            `json:"default"`
	Consumables uint32            `json:"consumables"`
	Categories  map[string]uint32 `json:"categories,omitempty"`
}

var (
	priceMu     sync.RWMutex
	activePrice = defaultPriceTable()
)

func defaultPriceTable() PriceTable {
	return PriceTable{
		Default:     LocalSugarPrice,
		Consumables: LocalConsumableSugarPrice,
		Categories: map[string]uint32{
			"food":     LocalConsumableSugarPrice,
			"item":     LocalConsumableSugarPrice,
			"skill":    LocalConsumableSugarPrice,
			"petcard":  LocalConsumableSugarPrice,
		},
	}
}

func DefaultPriceTable() PriceTable { return defaultPriceTable() }

func SetPriceTable(table PriceTable) {
	if table.Default == 0 {
		table.Default = LocalSugarPrice
	}
	if table.Consumables == 0 {
		table.Consumables = LocalConsumableSugarPrice
	}
	priceMu.Lock()
	activePrice = table
	priceMu.Unlock()
}

func ActivePriceTable() PriceTable {
	priceMu.RLock()
	defer priceMu.RUnlock()
	copyTable := activePrice
	if copyTable.Categories != nil {
		copyTable.Categories = make(map[string]uint32, len(activePrice.Categories))
		for key, value := range activePrice.Categories {
			copyTable.Categories[key] = value
		}
	}
	return copyTable
}

func LoadPriceTable(path string) (PriceTable, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return defaultPriceTable(), nil
		}
		return PriceTable{}, err
	}
	var table PriceTable
	if err = json.Unmarshal(data, &table); err != nil {
		return PriceTable{}, fmt.Errorf("解析价格配置 %s: %w", path, err)
	}
	if table.Default == 0 {
		table.Default = LocalSugarPrice
	}
	if table.Consumables == 0 {
		table.Consumables = LocalConsumableSugarPrice
	}
	return table, nil
}
