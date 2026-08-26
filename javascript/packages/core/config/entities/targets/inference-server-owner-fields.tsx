import { SelectField } from '#core/components/form/fields/select/select-field';
import { StringField } from '#core/components/form/fields/string/string-field';
import { useFormState } from '#core/components/form/hooks/use-form-state';
import { FormGroup } from '#core/components/form/layout/form-group/form-group';
import { required } from '#core/components/form/validation/validators';
import { TENANCY_TYPE } from './shared';

import type { InferenceServer } from './types';

const TIER_OPTIONS = [1, 2, 3, 4].map((tier) => ({ id: tier, label: `Tier ${tier}` }));

/**
 * Fields shown only for multi-tenant inference servers. Reads the current tenancy
 * type from form state rather than props, since it can change after mount.
 */
export function InferenceServerOwnerFields() {
  const { values } = useFormState<InferenceServer>({ values: true });
  if (values?.spec?.tenancyType !== TENANCY_TYPE.MULTI_TENANT) return null;

  return (
    <FormGroup title="Owner information">
      <StringField
        name="spec.ownerSpec.ownerInfo.owningTeam"
        label="Owning team"
        required
        validate={required()}
        description="uOwn asset UUID for the team that owns this inference server."
      />
      <StringField
        name="spec.ownerSpec.ownerInfo.owners"
        label="Owners"
        multi
        placeholder="Add an LDAP username and press Enter…"
      />
      <StringField
        name="spec.ownerSpec.ownerInfo.ownerGroups"
        label="Owner groups"
        multi
        placeholder="Add an LDAP group and press Enter…"
      />
      <SelectField
        name="spec.ownerSpec.tier"
        label="Tier"
        required
        validate={required()}
        options={TIER_OPTIONS}
        clearable={false}
      />
    </FormGroup>
  );
}
