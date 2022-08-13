import "./users.css";

import _ from 'lodash';
import React from 'react';
import PropTypes from 'prop-types';

import api from '../core/api-service.js';
import axios from 'axios';
import Select from 'react-select';
import T from '../i8n/i8n.js';

// FIXME - get the current username.
const username = "";

export default class UserForm extends React.Component {
    static propTypes = {
        value: PropTypes.array,
        onChange: PropTypes.func,
    };

    componentDidMount = () => {
        this.source = axios.CancelToken.source();
        this.loadUsers();
    }

    componentWillUnmount() {
        this.source.cancel("unmounted");
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
        api.get("v1/GetUsers", {}, this.source.token).then((response) => {
            let names = [];
            for(var i = 0; i<response.data.users.length; i++) {
                var name = response.data.users[i].name;

                // Only add other users than the currently logged in
                // user.
                if (name !== username) {
                    names.push({value: name, label: name});
                };
            }
            this.setState({options: names, initialized: true});
        });
    };

    handleChange = (newValue, actionMeta) => {
        this.props.onChange(_.map(newValue, x=>x.value));
    };

    render() {
        let default_value = _.map(this.props.value, x=>{
            return {value: x, label: x};});

        return (
            <Select
              isMulti
              isClearable
              value={default_value}
              className="users"
              classNamePrefix="velo"
              options={this.state.options}
              onChange={this.handleChange}
              placeholder={T("Select a user")}
            />
        );
    }
};
