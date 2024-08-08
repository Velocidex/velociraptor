import _ from 'lodash';

import PropTypes from 'prop-types';
import React, { Component, PureComponent } from 'react';
import UserConfig from '../core/user.jsx';
import Form from 'react-bootstrap/Form';
import Col from 'react-bootstrap/Col';
import Row from 'react-bootstrap/Row';
import Select from 'react-select';
import T from '../i8n/i8n.jsx';
import InputGroup from 'react-bootstrap/InputGroup';
import Button from 'react-bootstrap/Button';


export default class OrgSelector extends Component {
    static propTypes = {
        value: PropTypes.array,
        onChange: PropTypes.func,
    };

    static contextType = UserConfig;

    state = {
        options: [],
    }

    render() {
        // Check if the user has permissions to launch on different
        // orgs at all.
        let has_perm = this.context.traits &&
            this.context.traits.Permissions &&
            this.context.traits.Permissions.org_admin;

        let orgs = this.context.traits && this.context.traits.orgs;
        if (!has_perm || !orgs) {
            return <></>;
        }

        let options = _.map(this.context.traits.orgs, x=>{
            return {value: x.id, label: x.name,
                    isFixed: true, color: "#00B8D9"};
        });

        // Initial set of orgs
        let value = this.props.value;
        let option_value = _.filter(options, x=>{
            return _.find(value, y=>x.value===y);
        });

        return (
            <InputGroup className="mb-3">
              <Button
                className="btn btn-default"
                onClick={e=>{
                    this.props.onChange(_.map(orgs, x=>x.id));
                    e.preventDefault();
                    return false;
                }}
              >
                {T("All Orgs")}
              </Button>
              <Select
                className="org-selector"
                isMulti
                isClearable
                classNamePrefix="velo"
                options={options}
                value={option_value}
                onChange={x=>{
                    let value=_.map(x, x=>x.value);
                    if (value) {
                        return this.props.onChange(value);
                    };
                    return  [this.context.org || "root"];
                }}
                placeholder={T("Select an org")}
              />
            </InputGroup>
        );
    }
}



export class OrgSelectorForm extends PureComponent {
    static propTypes = {
        value: PropTypes.array,
        onChange: PropTypes.func,
    };

    render() {
        return (
            <Form.Group as={Row}>
              <Form.Label column sm="3">{T("Orgs")}</Form.Label>
              <Col sm="8">
                <OrgSelector
                  onChange={this.props.onChange}
                  value={this.props.value}/>
              </Col>
            </Form.Group>
        );
    }

}
