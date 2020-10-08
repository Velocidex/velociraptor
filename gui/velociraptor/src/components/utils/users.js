import React from 'react';
import PropTypes from 'prop-types';

import _ from 'lodash';

import api from '../core/api-service.js';
import MultiSelect from "@khanacademy/react-multi-select";

// FIXME - get the current username.
const username = "";

export default class UserForm extends React.Component {
    static propTypes = {
        value: PropTypes.array,
        onChange: PropTypes.func,
    };

    componentDidMount = () => {
        this.loadUsers();
    }

    componentDidUpdate = (prevProps, prevState, rootNode) => {
        if (!this.state.initialized) {
            this.loadUsers();
        }
    }

    state = {
        options: [],
        initialized: false,
    }

    loadUsers = () => {
        api.get("api/v1/GetUsers").then((response) => {
            let names = [];
            for(var i = 0; i<response.data.users.length; i++) {
                var name = response.data.users[i].name;

                // Only add other users than the currently logged in
                // user.
                if (name != username) {
                    names.push({value: name, label: name});
                };
            }
            this.setState({options: names, initialized: true});
        });
    };

    render() {
        return (
            <>
              <MultiSelect
                options={this.state.options}
                selected={this.props.value}
                onSelectedChanged={selected => this.props.onChange(selected)}
              />
            </>
        );
    }
};
