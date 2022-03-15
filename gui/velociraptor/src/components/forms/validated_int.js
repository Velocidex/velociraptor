import "./validated.css";
import _ from 'lodash';
import React from 'react';
import PropTypes from 'prop-types';

import Form from 'react-bootstrap/Form';

const regexp = new RegExp(`^-?[0-9]+$`);

export default class ValidatedInteger extends React.Component {
    static propTypes = {
        setInvalid: PropTypes.func,
        setValue: PropTypes.func.isRequired,
        value: PropTypes.any,
        placeholder: PropTypes.string,
        valid_func: PropTypes.func,
    };

    state = {
        invalid: false,
    }

    checkValue = value=>{
        let res = regexp.test(value);
        if (this.props.valid_func) {
            return res && this.props.valid_func(value);
        }
        return res;
    }

    render() {
        let value = this.props.value;

        // Need to set the initial value to '' to tell React this is a
        // controlled component.
        if (_.isUndefined(value) || _.isNaN(value)) {
            value = '';
        }

        return (
            <>
              <Form.Control placeholder={this.props.placeholder || ""}
                            className={ this.state.invalid && 'invalid' }
                            value={ value }
                            onChange={ (event) => {
                                const newValue = event.target.value;
                                let invalid = true;

                                // Value is allowed to be empty
                                if (_.isEmpty(newValue)) {
                                    invalid = false;
                                    this.props.setValue(undefined);

                                } else if (this.checkValue(newValue)) {
                                    this.props.setValue(parseInt(newValue));
                                    invalid = false;

                                } else {
                                    this.props.setValue(newValue);
                                    invalid = true;
                                }

                                if (this.props.setInvalid) {
                                    this.props.setInvalid(invalid);
                                }
                                this.setState({invalid: invalid});
                            } }
              />
            </>
        );
    }
};
