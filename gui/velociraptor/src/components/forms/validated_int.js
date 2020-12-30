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
    };

    state = {
        invalid: false,
    }

    render() {
        let value = this.props.value;
        if (_.isUndefined(value)) {
            value = 0;
        }
        return (
            <>
              <Form.Control placeholder={this.props.placeholder || ""}
                            className={ this.state.invalid && 'invalid' }
                            value={ value }
                            onChange={ (event) => {
                                const newValue = event.target.value;
                                let invalid = true;
                                if (regexp.test(newValue)) {
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
