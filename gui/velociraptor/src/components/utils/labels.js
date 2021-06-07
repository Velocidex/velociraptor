import React from 'react';
import PropTypes from 'prop-types';

import _ from 'lodash';

import api from '../core/api-service.js';
import CreatableSelect from 'react-select/creatable';


export default class LabelForm extends React.Component {
    static propTypes = {
        value: PropTypes.array,
        onChange: PropTypes.func,
    };

    componentDidMount = () => {
        this.loadLabels();
    }

    componentDidUpdate = (prevProps, prevState, rootNode) => {
        if (!this.state.initialized) {
            this.loadLabels();
        }
    }

    state = {
        options: [],
        initialized: false,
    }

    loadLabels = () => {
        api.get("v1/SearchClients", {
            query: "label:*",
            limit: 100,
            type: 1,
        }).then((response) => {
            let labels = _.map(response.data.names, (x) => {
                x = x.replace(/^label:/, "");
                return {value: x, label: x, color: "#00B8D9", isFixed: true};
            });
            this.setState({options: labels, initialized: true});
        });
    };

    handleChange = (newValue, actionMeta) => {
        this.props.onChange(_.map(newValue, x=>x.value));
    };
    render() {
        return (
            <>
              <CreatableSelect
                isMulti
                isClearable
                className="labels"
                classNamePrefix="velo"
                options={this.state.options}
                onChange={this.handleChange}
                placeholder="Select a label"
              />
            </>
        );
    }
};
