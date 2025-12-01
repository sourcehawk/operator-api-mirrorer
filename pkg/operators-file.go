package pkg

type OperatorsFile struct {
	Operators []OperatorConfig `yaml:"operators"`
}

func (op *OperatorsFile) Process(mirrorRootPath string, moduleRoot string, target string) error {
	for _, op := range op.Operators {
		if target == "" || target == op.Slug {
			err := op.Mirror(mirrorRootPath, moduleRoot)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func (op *OperatorsFile) Tag(mirrorRootPath string) (int, error) {
	createdCount := 0
	for _, op := range op.Operators {
		created, err := op.Tag(mirrorRootPath)
		if err != nil {
			return createdCount, err
		}
		if created {
			createdCount++
		}
	}

	return createdCount, nil
}
